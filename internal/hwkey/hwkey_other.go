//go:build !darwin && !windows

package hwkey

import "errors"

var errUnsupported = errors.New("not supported on this platform")

func available() error { return errUnsupported }

func create(string) (Signer, error) { return nil, errUnsupported }

func open(string) (Signer, error) { return nil, errUnsupported }

func remove(string) error { return errUnsupported }
