import unittest
from types import SimpleNamespace
from unittest.mock import patch

from shift_compacto import Remap, physical_keyboard, discover_keyboards


class RemapTests(unittest.TestCase):
    def test_toque_envia_barra_depois_de_liberar_shift(self):
        state = Remap()
        self.assertEqual(state.event(89, 1), [(54, 1)])
        self.assertEqual(state.event(89, 0), [(54, 0), (89, 1), (89, 0)])

    def test_shift_com_letra_nao_envia_barra(self):
        state = Remap()
        state.event(89, 1)
        self.assertEqual(state.event(30, 1), [(30, 1)])
        self.assertEqual(state.event(30, 0), [(30, 0)])
        self.assertEqual(state.event(89, 0), [(54, 0)])

    def test_shift_esquerdo_preserva_interrogacao(self):
        state = Remap()
        state.event(42, 1)
        self.assertEqual(state.event(89, 1), [(89, 1)])
        self.assertEqual(state.event(89, 2), [(89, 2)])
        self.assertEqual(state.event(89, 0), [(89, 0)])
        self.assertTrue(state.left_down)

    def test_repeticao_da_tecla_sozinha_envia_uma_barra(self):
        state = Remap()
        state.event(89, 1)
        self.assertEqual(state.event(89, 2), [])
        self.assertEqual(state.event(89, 1), [])
        self.assertEqual(state.event(89, 0), [(54, 0), (89, 1), (89, 0)])

    def test_proximo_toque_apos_uso_como_shift(self):
        state = Remap()
        state.event(89, 1)
        state.event(30, 1)
        state.event(89, 0)
        state.event(30, 0)
        state.event(89, 1)
        self.assertEqual(state.event(89, 0), [(54, 0), (89, 1), (89, 0)])

    def test_modo_normal_persiste_ao_soltar_shift_primeiro(self):
        state = Remap()
        state.event(42, 1)
        state.event(89, 1)
        state.event(42, 0)
        self.assertEqual(state.event(89, 0), [(89, 0)])


class DiscoveryTests(unittest.TestCase):
    def device(self, bus, keys, phys='physical'):
        return SimpleNamespace(
            name='Keyboard', path='/dev/input/event25',
            info=SimpleNamespace(bustype=bus), phys=phys,
            capabilities=lambda: {1: keys}, close=lambda: None,
        )

    def test_aceita_bluetooth_usb_e_interno(self):
        for bus in (5, 3, 0x11):
            with self.subTest(bus=bus):
                self.assertTrue(physical_keyboard(self.device(bus, [89, 42, 30]), 89, 42))

    def test_ignora_mouse_e_teclado_sem_tecla_alvo(self):
        for keys in ([272], [42, 30]):
            self.assertFalse(physical_keyboard(self.device(5, keys), 89, 42))

    def test_ignora_virtual_mesmo_se_anuncia_usb(self):
        from pathlib import Path
        with patch('shift_compacto.Path.resolve', return_value=Path('/sys/devices/virtual/input/input87')):
            self.assertFalse(physical_keyboard(self.device(3, [89, 42, 30]), 89, 42))

    def test_ignora_barramento_virtual_e_uinput(self):
        from evdev import ecodes
        self.assertFalse(physical_keyboard(self.device(ecodes.BUS_VIRTUAL, [89, 42, 30]), 89, 42))
        self.assertFalse(physical_keyboard(self.device(3, [89, 42, 30], 'py-evdev-uinput'), 89, 42))

    def test_descobre_todos_em_vez_de_escolher_apenas_um(self):
        devices = {path: self.device(bus, [89, 42, 30]) for path, bus in [('event17', 5), ('event4', 0x11)]}
        with patch('evdev.list_devices', return_value=list(devices)), patch('evdev.InputDevice', side_effect=devices.__getitem__):
            self.assertEqual(discover_keyboards(89, 42), [('event17', 'Keyboard'), ('event4', 'Keyboard')])


if __name__ == '__main__':
    unittest.main()
