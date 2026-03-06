#ifndef __RTG_SIGNAL_H
#define __RTG_SIGNAL_H

typedef void (*sighandler_t)(int);

#define SIGINT 2
#define SIGTERM 15

sighandler_t signal(int sig, sighandler_t handler);
int raise(int sig);

#endif
