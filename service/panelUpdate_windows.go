//go:build windows

package service

import "errors"

func startDetached(string, []string) error {
	return errors.New("the panel does not update itself on Windows")
}
