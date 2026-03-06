#ifndef __RTG_GETOPT_H
#define __RTG_GETOPT_H

extern char *optarg;
extern int optind;
extern int opterr;
extern int optopt;

int getopt(int argc, char *const argv[], const char *optstring);
int getopt_long(int argc, char *const argv[], const char *optstring,
                const void *longopts, int *longindex);
int getopt_long_only(int argc, char *const argv[], const char *optstring,
                     const void *longopts, int *longindex);

#endif
