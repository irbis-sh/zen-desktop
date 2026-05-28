package process

/*
#cgo LDFLAGS: -lproc

#include <stdint.h>
#include <string.h>
#include <sys/types.h>

// Defined in process_darwin.c
int find_pid_by_port(uint16_t port, pid_t *out_pid);
*/
import "C"

import "fmt"

func findPIDBySourcePort(port uint16) (PID, error) {
	if port == 0 {
		return 0, ErrNotFound
	}

	var pid C.pid_t
	ret := C.find_pid_by_port(C.uint16_t(port), &pid)
	switch {
	case ret == 1:
		return 0, ErrNotFound
	case ret < 0:
		return 0, fmt.Errorf("find pid for port %d: %s", port, C.GoString(C.strerror(-ret)))
	}

	return PID(pid), nil
}
