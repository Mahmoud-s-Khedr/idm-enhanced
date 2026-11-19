# Synthetic DASH fixture

These checked-in two-second H.264/AAC fragmented MP4 objects contain generated
video test patterns and a generated sine wave, with no third-party media content.
They provide 128x72 and 256x144 video plus two explicit audio choices. The static
manifest uses Number templates and finite timelines; the second audio AdaptationSet
is labeled fra to exercise selection independently from eng. Both tones are 440 Hz;
tests prove track identity by asserting only selected initialization URLs download.

Generation (FFmpeg 9.0.2 with libx264 and AAC encoder):

```sh
ffmpeg -nostdin -hide_banner -loglevel error \
  -f lavfi -i testsrc2=size=256x144:rate=10 \
  -f lavfi -i sine=frequency=440:sample_rate=44100 \
  -filter_complex '[0:v]split=2[hi][l];[l]scale=128:72[lo]' \
  -map '[lo]' -map '[hi]' -map 1:a -map 1:a \
  -c:v libx264 -preset ultrafast -profile:v baseline -g 10 -bf 0 \
  -c:a aac -b:a 48k -t 2 \
  -metadata:s:a:0 language=eng -metadata:s:a:1 language=fra \
  -f dash -seg_duration 1 -use_template 1 -use_timeline 1 \
  -adaptation_sets 'id=0,streams=v id=1,streams=a' \
  -init_seg_name 'init-$RepresentationID$.mp4' \
  -media_seg_name 'chunk-$RepresentationID$-$Number%05d$.m4s' \
  testdata/dash/manifest.mpd
```

After generation, split Representation 3 into its own audio AdaptationSet with
lang=fra. The remaining generated metadata is preserved. Mandatory media-smoke
checks all four combinations with real FFmpeg/ffprobe and full decode, including
missing tools, persisted reopen, offline assembly and completed-file preservation.
