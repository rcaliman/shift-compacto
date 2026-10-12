# Shift para teclado compacto — Go

Versão independente do Python, para Linux e Windows de 64 bits (amd64 ou arm64). Usa somente a biblioteca padrão do Go; não requer `keyboard`, `evdev`, CGO ou downloads de dependências.

- Toque na tecla 89: digita `/` ao soltar, se nenhuma outra tecla foi pressionada durante o toque.
- Segurar 89 e pressionar outra tecla: funciona como Shift direito, sem digitar `/`.
- Shift esquerdo (42) primeiro e depois 89: digita `?`, respeitando o layout do sistema.
- Ctrl+C no terminal: encerra e libera as teclas virtuais.

Aplica a todos os teclados físicos compatíveis, sem salvar configuração. Os códigos padrão são 89 e 42; ajuste com `--key` e `--left-shift` se necessário.

## Linux

O executável `shift-compacto` já foi compilado para Linux amd64 nesta pasta:

```bash
cd ~/Projetos/shift-compacto
sudo ./shift-compacto
```

Encerre a versão Python antes de iniciar a versão Go. Use apenas uma instância do remapeador por vez.

No Linux, exclui dispositivos virtuais. Na execução manual padrão, detecta os teclados ao iniciar; reinicie após conectar ou reconectar um teclado, ou use `--watch` para acompanhar mudanças automaticamente. Cada teclado possui estado independente: use a tecla 89 e as letras no mesmo teclado.

Comandos opcionais:

```bash
sudo ./shift-compacto --list
sudo ./shift-compacto --check
sudo ./shift-compacto --device /dev/input/eventX
./shift-compacto --help
```

`--check` cria e remove os teclados virtuais para verificar permissões e compatibilidade com `uinput`, sem capturar teclas.

Para recompilar, instale Go 1.24 ou superior e execute:

```bash
go build -buildvcs=false -o shift-compacto .
```

O Linux exige acesso a `/dev/input/event*` e `/dev/uinput`; os exemplos usam `sudo`. A implementação utiliza [evdev e uinput do kernel](https://kernel.org/doc/html/latest/input/uinput.html).

## Windows

Copie `shift-compacto.exe` (já compilado para Windows amd64) e execute em PowerShell na pasta do arquivo:

```powershell
.\shift-compacto.exe
```

Não precisa instalar Python, Go nem bibliotecas para executar o binário. Normalmente funciona sem administrador. Para atuar em aplicativos executados como administrador, abra o terminal do remapeador como administrador também.

Para compilar no próprio Windows com Go instalado, copie `go.mod` e todos os arquivos `.go` para a mesma pasta e execute:

```powershell
go build -buildvcs=false -o shift-compacto.exe .
```

Para compilar para Windows a partir do Linux:

```bash
GOOS=windows GOARCH=amd64 go build -buildvcs=false -o shift-compacto.exe .
```

Usa um [hook global do Windows](https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-setwindowshookexw) e [SendInput](https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-sendinput); ignora os eventos que ele próprio injeta. Aplica a todos os teclados. Eventos de teclados diferentes compartilham o estado no Windows.

## Testes

```bash
go test -race ./...
go vet ./...
```

A lógica de toque, Shift, repetição e ordem de soltura foi testada. Os executáveis foram compilados para Linux e Windows amd64. O funcionamento com teclado físico no Windows precisa ser validado nesse sistema.

## Serviço do sistema no Linux

O executável foi instalado em `/usr/bin/shift-compacto`. O serviço `shift-compacto.service` inicia automaticamente com o sistema e executa `shift-compacto --watch` como root, com acesso aos dispositivos de entrada.

Nesse modo, o script acompanha conexão e reconexão de teclados. Ao mudar o conjunto de teclados, libera a captura e reabre os dispositivos. Se houver teclas pressionadas, aguarda que sejam soltas antes de retomar.

Não execute outra instância Python ou Go enquanto o serviço estiver ativo.

Consultar o estado e os logs:

```bash
systemctl status shift-compacto.service
journalctl -u shift-compacto.service -n 30 --no-pager
```

Parar temporariamente ou reiniciar:

```bash
sudo systemctl stop shift-compacto.service
sudo systemctl restart shift-compacto.service
```

Desativar a inicialização automática e parar:

```bash
sudo systemctl disable --now shift-compacto.service
```

Reativar:

```bash
sudo systemctl enable --now shift-compacto.service
```

Arquivos de implantação no projeto: `shift-compacto.service` e `install-service.sh`. O instalador recusa substituir uma instalação existente; futuras atualizações precisam substituir o binário e reiniciar o serviço.
