default: selfhost

build:
  sh mkdir -p ../build
  sh go build -o ../build/rtg ../std/compiler/

selfhost: build
  sh ../build/rtg -strict -o ../build/stage1 ../std/compiler/
  sh ../build/stage1 -strict -o ../build/stage_out ../std/compiler/ && mv ../build/stage_out ../build/stage2
  sh ../build/stage2 -strict -o ../build/stage_out ../std/compiler/ && mv ../build/stage_out ../build/stage3
  sh cmp ../build/stage2 ../build/stage3 && echo "PASS: self-hosting OK"
