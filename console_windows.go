package main

import (
	"bufio"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Liga as sequências ANSI no console do Windows (as cores do -ver-dmx); o
// console antigo só as mostra se pedir. Os acentos já saem certos: o Go
// escreve no console em UTF-16.
func init() {
	for _, h := range []windows.Handle{windows.Stdout, windows.Stderr} {
		var mode uint32
		if windows.GetConsoleMode(h, &mode) == nil {
			windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
		}
	}
}

var getConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// waitBeforeClosing segura a janela quando o programa foi aberto com dois
// cliques (a janela do console é só dele) e vai fechar com erro: sem isso,
// ela some antes de dar para ler a mensagem. Num terminal, não espera.
func waitBeforeClosing() {
	ids := make([]uint32, 4)
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	if n != 1 {
		return
	}
	fmt.Fprint(os.Stderr, "\nAperte Enter para fechar.")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
