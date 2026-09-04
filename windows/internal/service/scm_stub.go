//go:build !windows

package service

import "fmt"

func Install(string) error   { return fmt.Errorf("install only supported on Windows") }
func Uninstall() error       { return fmt.Errorf("uninstall only supported on Windows") }
func Start() error           { return fmt.Errorf("start only supported on Windows") }
func Stop() error            { return fmt.Errorf("stop only supported on Windows") }
func QueryState() (string, error) {
	return "not_windows", nil
}
