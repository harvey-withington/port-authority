//go:build !windows

package main

import "errors"

func runETWSpike([]string) error {
	return errors.New("etw-spike is Windows only")
}
