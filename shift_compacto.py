#!/usr/bin/env python3
"""Toque em 89 digita /; 89 + outra tecla funciona como Shift.
Shift esquerdo + 89 preserva a tecla original para digitar ?.

Windows: pip install keyboard
Linux (Fedora): sudo dnf install python3-evdev
         sudo /usr/bin/python3 ~/Projetos/shift-compacto/shift_compacto.py --list
         sudo /usr/bin/python3 ~/Projetos/shift-compacto/shift_compacto.py

Aplica a todos os teclados físicos conectados ao iniciar, sem salvar configuração.
No Linux, reinicie após conectar ou reconectar um teclado.
Encerre com Ctrl+C. Não instala serviço nem altera o layout do sistema.
O modo escolhido ao pressionar 89 permanece até soltá-la.
Sem outra tecla pressionada durante esse intervalo, / é enviado ao soltar 89.
"""

import argparse
import errno
import platform
import signal
import threading
from contextlib import ExitStack, closing, contextmanager
from pathlib import Path
from select import select


class Remap:
    """Transforma eventos: value 0=soltar, 1=pressionar, 2=repetir."""

    def __init__(self, target=89, left=42, right=54):
        self.target, self.left, self.right = target, left, right
        self.left_down = False
        self.mode = None
        self.used = False

    def event(self, code, value):
        if code == self.left:
            self.left_down = value != 0
        if code != self.target:
            if self.mode == 'shift' and value != 0:
                self.used = True
            return [(code, value)]
        if value == 0:
            mode, self.mode = self.mode, None
            if mode == 'shift':
                output = [(self.right, 0)]
                if not self.used:
                    output.extend([(code, 1), (code, 0)])
                return output
            return [(code, 0)] if mode == 'normal' else []
        if self.mode is None:
            self.used = False
            self.mode = 'normal' if self.left_down else 'shift'
            return [(code if self.mode == 'normal' else self.right, 1)]
        return [(code, value)] if self.mode == 'normal' else []


def windows(args):
    import keyboard

    state = Remap(args.key, args.left_shift)
    state.left_down = keyboard.is_pressed(args.left_shift)
    stop = threading.Event()
    lock = threading.RLock()

    def callback(event):
        with lock:
            value = 0 if event.event_type == keyboard.KEY_UP else 1
            previous_mode = state.mode
            output = state.event(event.scan_code, value)
            if event.scan_code != args.key:
                return True
            if previous_mode == 'normal' or state.mode == 'normal':
                return True  # Preserva /? com o Shift esquerdo físico.
            for code, action in output:
                if action:
                    keyboard.press(code)
                else:
                    keyboard.release(code)
            return False  # Bloqueia /? quando está funcionando como Shift.

    handlers = []
    try:
        handlers.append(keyboard.hook(callback, suppress=True))
        print('Ativo. Ctrl+C neste terminal para encerrar.', flush=True)
        stop.wait()
    finally:
        with lock:
            for handler in handlers:
                keyboard.unhook(handler)
            if state.mode == 'shift':
                keyboard.release(state.right)


def physical_keyboard(device, key, left_shift):
    """Exclui dispositivos uinput mesmo quando anunciam um barramento físico."""
    from evdev import ecodes

    keys = set(device.capabilities().get(ecodes.EV_KEY, []))
    if not {key, left_shift, ecodes.KEY_A} <= keys:
        return False
    sysfs = Path('/sys/class/input') / Path(device.path).name / 'device'
    # Dispositivos Bluetooth UHID também ficam sob /virtual/, mas não
    # sob /devices/virtual/input/, onde ficam os teclados uinput.
    if '/devices/virtual/input/' in str(sysfs.resolve()):
        return False
    return device.info.bustype != ecodes.BUS_VIRTUAL and device.phys != 'py-evdev-uinput'


def discover_keyboards(key, left_shift):
    from evdev import InputDevice, list_devices

    candidates = []
    for path in sorted(list_devices()):
        try:
            with closing(InputDevice(path)) as device:
                if physical_keyboard(device, key, left_shift):
                    candidates.append((path, device.name))
        except FileNotFoundError:
            continue
    return candidates


@contextmanager
def grab_keyboard(device):
    device.grab()
    try:
        yield
    finally:
        try:
            device.ungrab()
        except OSError as exc:
            if exc.errno != errno.ENODEV:
                raise


def linux(args):
    from evdev import InputDevice, UInput, ecodes

    candidates = discover_keyboards(args.key, args.left_shift)
    if args.list:
        for path, name in candidates:
            print(f'{path}: {name}')
        if not candidates:
            print('Nenhum teclado físico compatível acessível. Tente executar com sudo.')
        return
    paths = [args.device] if args.device else [path for path, name in candidates]
    if not paths:
        raise SystemExit('Nenhum teclado físico compatível acessível. Execute com sudo e conecte o teclado.')

    # Um estado e um teclado virtual por dispositivo físico evitam misturar
    # pressionamentos simultâneos vindos de teclados diferentes.
    devices = {}
    with ExitStack() as stack:
        for path in paths:
            device = stack.enter_context(closing(InputDevice(path)))
            if not physical_keyboard(device, args.key, args.left_shift):
                raise SystemExit(f'{path} não é um teclado físico compatível.')
            if device.active_keys():
                raise SystemExit(f'Solte todas as teclas de {device.name} antes de iniciar.')
            caps = device.capabilities()
            caps.pop(ecodes.EV_SYN, None)
            caps.pop(ecodes.EV_FF, None)
            caps[ecodes.EV_KEY] = sorted(set(caps[ecodes.EV_KEY]) | {54})
            virtual = stack.enter_context(UInput(caps, name=f'Shift compacto virtual: {device.name}'))
            devices[device.fd] = (device, virtual, Remap(args.key, args.left_shift), set())
        for device, virtual, state, pressed in devices.values():
            stack.enter_context(grab_keyboard(device))
            print(f'Ativo em {device.name} ({device.path})', flush=True)
        print('Ctrl+C neste terminal para encerrar.', flush=True)
        try:
            while devices:
                ready, _, _ = select(list(devices), [], [])
                for fd in ready:
                    device, virtual, state, pressed = devices[fd]
                    try:
                        events = list(device.read())
                    except BlockingIOError:
                        continue
                    except OSError as exc:
                        # Ao desconectar, libera as teclas desse dispositivo.
                        if exc.errno != errno.ENODEV:
                            raise
                        for code in pressed:
                            virtual.write(ecodes.EV_KEY, code, 0)
                        virtual.syn()
                        pressed.clear()
                        del devices[fd]
                        print(f'Desconectado: {device.name}. Reinicie para detectar teclados reconectados.', flush=True)
                        continue
                    for event in events:
                        if event.type == ecodes.EV_SYN and event.code == ecodes.SYN_DROPPED:
                            raise RuntimeError('Eventos perdidos; reinicie o script.')
                        if event.type == ecodes.EV_KEY:
                            for code, value in state.event(event.code, event.value):
                                virtual.write(ecodes.EV_KEY, code, value)
                                if value:
                                    pressed.add(code)
                                else:
                                    pressed.discard(code)
                        else:
                            virtual.write_event(event)
        finally:
            for device, virtual, state, pressed in devices.values():
                for code in pressed:
                    virtual.write(ecodes.EV_KEY, code, 0)
                virtual.syn()


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--key', type=int, default=89, help='Código da tecla /? (89).')
    parser.add_argument('--left-shift', type=int, default=42, help='Código do Shift esquerdo (42).')
    parser.add_argument('--list', action='store_true', help='Lista teclados físicos compatíveis no Linux.')
    parser.add_argument('--device', help='Restringe a um dispositivo no Linux (padrão: todos os teclados físicos).')
    args = parser.parse_args()
    if len({args.key, args.left_shift, 54}) != 3:
        parser.error('Os códigos da tecla, Shift esquerdo e Shift direito (54) devem ser distintos.')

    def terminate(signum, frame):
        raise KeyboardInterrupt

    signal.signal(signal.SIGTERM, terminate)
    try:
        if platform.system() == 'Windows':
            windows(args)
        elif platform.system() == 'Linux':
            linux(args)
        else:
            parser.error('Suporte apenas a Windows e Linux.')
    except KeyboardInterrupt:
        print('\nEncerrado; teclado liberado.')
    except ModuleNotFoundError as exc:
        parser.exit(1, f'Instale a dependência: {exc.name}. Veja --help.\n')
    except (OSError, RuntimeError) as exc:
        parser.exit(1, f'Erro: {exc}\nNo Linux, verifique as permissões de /dev/input e /dev/uinput (execute com sudo).\n')


if __name__ == '__main__':
    main()
