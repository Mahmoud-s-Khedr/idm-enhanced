# Synthetic HLS playback fixtures

Generated locally with FFmpeg 9.0.2 from lavfi testsrc2 and sine sources. There is
no downloaded, copyrighted or personal video. Two two-second H.264 constrained
baseline/AAC-LC variants exercise selection and actual decoding after assembly.
MPEG-TS bytes are binary data even though their suffix is .ts.

For low/high respectively, run the following with SIZE=128x72 / 256x144,
FREQUENCY=800 / 1000, BITRATE=120k / 220k and OUTPUT=low / high:

```sh
ffmpeg -hide_banner -loglevel error \
  -f lavfi -i testsrc2=size=SIZE:rate=10 \
  -f lavfi -i sine=frequency=FREQUENCY:sample_rate=44100 \
  -t 2 -c:v libx264 -profile:v baseline -level:v 3.0 -pix_fmt yuv420p \
  -threads 1 -g 10 -keyint_min 10 -sc_threshold 0 -b:v BITRATE \
  -c:a aac -b:a 48k -map_metadata -1 -f hls -hls_time 1 \
  -hls_list_size 0 -hls_playlist_type vod \
  -hls_segment_filename OUTPUT/segment-%02d.ts OUTPUT/index.m3u8
```

The master declares the actual peak segment bytes × 8 (segments are one second)
and resolution of each generated variant. Generation requires an H.264 encoder;
mandatory playback tests use checked-in bytes and require no encoder or network.
Tests never regenerate fixtures or download media from public services.
