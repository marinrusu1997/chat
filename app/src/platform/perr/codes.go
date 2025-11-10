package perr

// POSIX error code constants.
const (
	E2BIG           string = "E2BIG"
	EACCES          string = "EACCES"
	EADDRINUSE      string = "EADDRINUSE"
	EADDRNOTAVAIL   string = "EADDRNOTAVAIL"
	EAFNOSUPPORT    string = "EAFNOSUPPORT"
	EAGAIN          string = "EAGAIN"
	EALREADY        string = "EALREADY"
	EAUTH           string = "EAUTH"
	EBADF           string = "EBADF"
	EBADMSG         string = "EBADMSG"
	EBADRPC         string = "EBADRPC"
	EBUSY           string = "EBUSY"
	ECANCELED       string = "ECANCELED"
	ECONFIG         string = "ECONFIG"
	ECHILD          string = "ECHILD"
	ECONNABORTED    string = "ECONNABORTED"
	ECONNREFUSED    string = "ECONNREFUSED"
	ECONNRESET      string = "ECONNRESET"
	EDEADLK         string = "EDEADLK"
	EDESTADDRREQ    string = "EDESTADDRREQ"
	EDOM            string = "EDOM"
	EEXIST          string = "EEXIST"
	EFAULT          string = "EFAULT"
	EFBIG           string = "EFBIG"
	EHOSTDOWN       string = "EHOSTDOWN"
	EHOSTUNREACH    string = "EHOSTUNREACH"
	EIDRM           string = "EIDRM"
	EILSEQ          string = "EILSEQ"
	EINIT           string = "EINIT"
	EINPROGRESS     string = "EINPROGRESS"
	EINTR           string = "EINTR"
	EINVAL          string = "EINVAL"
	EIO             string = "EIO"
	EISCONN         string = "EISCONN"
	EISDIR          string = "EISDIR"
	ELOOP           string = "ELOOP"
	EMFILE          string = "EMFILE"
	EMLINK          string = "EMLINK"
	EMSGSIZE        string = "EMSGSIZE"
	ENAMETOOLONG    string = "ENAMETOOLONG"
	ENETDOWN        string = "ENETDOWN"
	ENETRESET       string = "ENETRESET"
	ENETUNREACH     string = "ENETUNREACH"
	ENFILE          string = "ENFILE"
	ENOBUFS         string = "ENOBUFS"
	ENODEV          string = "ENODEV"
	ENOENT          string = "ENOENT"
	ENOEXEC         string = "ENOEXEC"
	ENOMEM          string = "ENOMEM"
	ENOPROTOOPT     string = "ENOPROTOOPT"
	ENOSPC          string = "ENOSPC"
	ENOSYS          string = "ENOSYS"
	ENOTCONN        string = "ENOTCONN"
	ENOTDIR         string = "ENOTDIR"
	ENOTEMPTY       string = "ENOTEMPTY"
	ENOTSOCK        string = "ENOTSOCK"
	ENOTSUP         string = "ENOTSUP"
	ENXIO           string = "ENXIO"
	EOPNOTSUPP      string = "EOPNOTSUPP"
	EOVERFLOW       string = "EOVERFLOW"
	EPERM           string = "EPERM"
	EPIPE           string = "EPIPE"
	EPROTO          string = "EPROTO"
	EPROTONOSUPPORT string = "EPROTONOSUPPORT"
	EPROTOTYPE      string = "EPROTOTYPE"
	ERANGE          string = "ERANGE"
	EROFS           string = "EROFS"
	ESHUTDOWN       string = "ESHUTDOWN"
	ESPIPE          string = "ESPIPE"
	ESRCH           string = "ESRCH"
	ETIMEDOUT       string = "ETIMEDOUT"
	EWOULDBLOCK     string = "EWOULDBLOCK"
	EXDEV           string = "EXDEV"
)

// Descriptions maps each error code to a human-readable message.
var Descriptions = map[string]string{
	E2BIG:           "Argument list too long",
	EACCES:          "Permission denied",
	EADDRINUSE:      "Address already in use",
	EADDRNOTAVAIL:   "Can’t assign requested address",
	EAFNOSUPPORT:    "Address family not supported by protocol family",
	EAGAIN:          "Resource temporarily unavailable",
	EALREADY:        "Operation already in progress",
	EAUTH:           "Authentication error",
	EBADF:           "Bad file descriptor",
	EBADMSG:         "Bad message",
	EBADRPC:         "RPC struct is bad",
	EBUSY:           "Device busy",
	ECANCELED:       "Operation canceled",
	ECONFIG:         "Configuration failure",
	ECHILD:          "No child processes",
	ECONNABORTED:    "Software caused connection abort",
	ECONNREFUSED:    "Connection refused",
	ECONNRESET:      "Connection reset by peer",
	EDEADLK:         "Resource deadlock avoided",
	EDESTADDRREQ:    "Destination address required",
	EDOM:            "Numerical argument out of domain",
	EEXIST:          "File exists",
	EFAULT:          "Bad address",
	EFBIG:           "File too large",
	EHOSTDOWN:       "Host is down",
	EHOSTUNREACH:    "No route to host",
	EIDRM:           "Identifier removed",
	EILSEQ:          "Illegal byte sequence",
	EINIT:           "Initialization failure",
	EINPROGRESS:     "Operation now in progress",
	EINTR:           "Interrupted system call",
	EINVAL:          "Invalid argument",
	EIO:             "Input/output error",
	EISCONN:         "Socket is already connected",
	EISDIR:          "Is a directory",
	ELOOP:           "Too many levels of symbolic links",
	EMFILE:          "Too many open files",
	EMLINK:          "Too many links",
	EMSGSIZE:        "Message too long",
	ENAMETOOLONG:    "File name too long",
	ENETDOWN:        "Network is down",
	ENETRESET:       "Network dropped connection on reset",
	ENETUNREACH:     "Network is unreachable",
	ENFILE:          "Too many open files in system",
	ENOBUFS:         "No buffer space available",
	ENODEV:          "Operation not supported by device",
	ENOENT:          "No such file or directory",
	ENOEXEC:         "Exec format error",
	ENOMEM:          "Cannot allocate memory",
	ENOPROTOOPT:     "Protocol not available",
	ENOSPC:          "No space left on device",
	ENOSYS:          "Function not implemented",
	ENOTCONN:        "Socket is not connected",
	ENOTDIR:         "Not a directory",
	ENOTEMPTY:       "Directory not empty",
	ENOTSOCK:        "Socket operation on non-socket",
	ENOTSUP:         "Operation not supported",
	ENXIO:           "Device not configured",
	EOPNOTSUPP:      "Operation not supported",
	EOVERFLOW:       "Value too large to be stored in data type",
	EPERM:           "Operation not permitted",
	EPIPE:           "Broken pipe",
	EPROTO:          "Protocol error",
	EPROTONOSUPPORT: "Protocol not supported",
	EPROTOTYPE:      "Protocol wrong type for socket",
	ERANGE:          "Result too large",
	EROFS:           "Read-only filesystem",
	ESHUTDOWN:       "Can’t send after socket shutdown",
	ESPIPE:          "Illegal seek",
	ESRCH:           "No such process",
	ETIMEDOUT:       "Operation timed out",
	EWOULDBLOCK:     "Resource temporarily unavailable",
	EXDEV:           "Cross-device link",
}

// Description returns a human-readable description for a POSIX code.
func Description(code string) string {
	if desc, ok := Descriptions[code]; ok {
		return desc
	}
	return "Unknown error"
}
