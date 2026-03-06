#include <arpa/inet.h>
#include <assert.h>
#include <complex.h>
#include <dirent.h>
#include <err.h>
#include <float.h>
#include <getopt.h>
#include <libgen.h>
#include <locale.h>
#include <netdb.h>
#include <netinet/in.h>
#include <pthread.h>
#include <setjmp.h>
#include <strings.h>
#include <sys/select.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <wchar.h>
#include <zlib.h>

int main(void) {
  return sizeof(jmp_buf) == 0 ||
         sizeof(struct dirent) == 0 ||
         sizeof(struct lconv) == 0 ||
         sizeof(struct sockaddr_in) == 0 ||
         sizeof(struct sockaddr_un) == 0 ||
         sizeof(struct sockaddr_storage) == 0 ||
         sizeof(struct addrinfo) == 0 ||
         sizeof(struct stat) == 0 ||
         sizeof(fd_set) == 0 ||
         sizeof(mbstate_t) == 0 ||
         sizeof(z_stream) == 0;
}
