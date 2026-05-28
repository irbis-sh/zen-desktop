//go:build darwin

#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <arpa/inet.h>
#include <libproc.h>

#define PIDS_INCR           (sizeof(int) * 32)
#define LIST_PIDS_RETRY_CNT 10

static int list_pids(pid_t **pids, size_t *count) {
	int nb;
	size_t buf_size = 0;

	if (!pids || !count || *pids) return -EINVAL;

	if ((nb = proc_listpids(PROC_ALL_PIDS, 0, NULL, 0)) <= 0) {
		return -errno;
	}

	while ((size_t)nb > buf_size) {
		buf_size += PIDS_INCR;
	}

	*pids = malloc(buf_size);
	if (!*pids) {
		return -ENOMEM;
	}

	int i;
	for (i = 0; i < LIST_PIDS_RETRY_CNT; i++) {
		if ((nb = proc_listpids(PROC_ALL_PIDS, 0, *pids, (int)buf_size)) <= 0) {
			free(*pids);
			*pids = NULL;
			return -errno;
		}

		if ((size_t)nb + sizeof(int) < buf_size) {
			*count = nb / sizeof(int);
			break;
		}

		buf_size += PIDS_INCR;
		pid_t *tmp = realloc(*pids, buf_size);
		if (!tmp) {
			free(*pids);
			*pids = NULL;
			return -ENOMEM;
		}
		*pids = tmp;
	}
	if (i == LIST_PIDS_RETRY_CNT) {
		free(*pids);
		*pids = NULL;
		return -EAGAIN;
	}

	return 0;
}

// find_pid_by_port looks up the PID owning the given TCP source port.
// Returns: 0 = success (PID written to *out_pid), 1 = not found, negative = -errno.
int find_pid_by_port(uint16_t port, pid_t *out_pid) {
	pid_t *pids = NULL;
	size_t pid_count;
	int err = list_pids(&pids, &pid_count);
	if (err != 0) return err;

	// Pre-convert to network byte order.
	uint16_t net_port = htons(port);

	// Reusable FD buffer, grown as needed across PIDs.
	size_t fds_bufsize = sizeof(struct proc_fdinfo) * 256;
	struct proc_fdinfo *fds = malloc(fds_bufsize);
	if (!fds) {
		free(pids);
		return -ENOMEM;
	}

	for (size_t i = 0; i < pid_count; ++i) {
		int pid = pids[i];
		if (pid <= 0) continue;

		// Call PROC_PIDLISTFDS directly, skipping PROC_PIDTASKALLINFO.
		// If the buffer is exactly full, it may have been too small - grow and retry.
		int nb;
		for (;;) {
			nb = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, fds, (int)fds_bufsize);
			if (nb <= 0) break;
			if ((size_t)nb + sizeof(struct proc_fdinfo) < fds_bufsize) break;
			fds_bufsize *= 2;
			struct proc_fdinfo *tmp = realloc(fds, fds_bufsize);
			if (!tmp) {
				free(fds);
				free(pids);
				return -ENOMEM;
			}
			fds = tmp;
		}
		if (nb <= 0) {
			if (errno == EPERM || errno == ESRCH) continue;
			free(fds);
			free(pids);
			return -errno;
		}

		int nf = nb / sizeof(struct proc_fdinfo);
		for (int j = 0; j < nf; j++) {
			if (fds[j].proc_fdtype != PROX_FDTYPE_SOCKET) continue;

			struct socket_fdinfo si;
			nb = proc_pidfdinfo(pid, fds[j].proc_fd, PROC_PIDFDSOCKETINFO, &si, sizeof(si));
			if (nb <= 0) {
				if (errno == EBADF || errno == ENOTSUP || errno == ESRCH) continue;
				free(fds);
				free(pids);
				return -errno;
			}
			if ((size_t)nb < sizeof(si)) continue;

			if (si.psi.soi_kind != SOCKINFO_TCP) continue;
			if (si.psi.soi_proto.pri_tcp.tcpsi_ini.insi_lport != net_port) continue;

			// Match found.
			*out_pid = pid;
			free(fds);
			free(pids);
			return 0;
		}
	}

	free(fds);
	free(pids);
	return 1; // not found
}

// find_process_path_by_pid resolves the filesystem path for a PID.
// Returns: 0 = success, negative = -errno.
int find_process_path_by_pid(pid_t pid, char *buf, size_t buflen) {
	int ret = proc_pidpath(pid, buf, (uint32_t)buflen);
	if (ret <= 0) return -errno;
	return 0;
}

// find_process_name_by_pid resolves the process name for a PID.
// Returns: 0 = success, negative = -errno.
int find_process_name_by_pid(pid_t pid, char *buf, size_t buflen) {
	int ret = proc_name(pid, buf, (uint32_t)buflen);
	if (ret <= 0) return -errno;
	return 0;
}
