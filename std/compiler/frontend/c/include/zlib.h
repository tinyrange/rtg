#ifndef __RTG_ZLIB_H
#define __RTG_ZLIB_H

#include <stddef.h>

typedef unsigned char Bytef;
typedef unsigned int uInt;
typedef unsigned long uLong;
typedef void *voidpf;
typedef char *charpf;

typedef struct z_stream_s {
  Bytef *next_in;
  uInt avail_in;
  uLong total_in;
  Bytef *next_out;
  uInt avail_out;
  uLong total_out;
  char *msg;
  void *state;
  voidpf zalloc;
  voidpf zfree;
  voidpf opaque;
  int data_type;
  uLong adler;
  uLong reserved;
} z_stream;

typedef z_stream *z_streamp;

#define Z_NO_FLUSH 0
#define Z_FINISH 4
#define Z_OK 0
#define Z_STREAM_END 1
#define Z_DEFAULT_COMPRESSION (-1)
#define MAX_WBITS 15
#define ZLIB_VERSION "1.2.11"

const char *zlibVersion(void);
uLong compressBound(uLong sourceLen);
int compress(Bytef *dest, uLong *destLen, const Bytef *source, uLong sourceLen);
int compress2(Bytef *dest, uLong *destLen, const Bytef *source, uLong sourceLen, int level);
int uncompress(Bytef *dest, uLong *destLen, const Bytef *source, uLong sourceLen);
int deflateInit_(z_streamp strm, int level, const char *version, int stream_size);
int deflate(z_streamp strm, int flush);
int deflateEnd(z_streamp strm);
int inflateInit_(z_streamp strm, const char *version, int stream_size);
int inflate(z_streamp strm, int flush);
int inflateEnd(z_streamp strm);

#endif
