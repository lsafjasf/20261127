package tailtrace

func int64Ptr(v int64) *int64 { return &v }

// SampleEvidence returns the canonical reproduction: 30 fps CFR, ten content
// frames, then two contiguous black frames appended by the renderer.
func SampleEvidence() Evidence {
	fps := MustRational(30, 1)
	timeBase := MustRational(1, 30)
	contentFrames := int64(10)
	contractFrames := contentFrames
	contractDuration := MustRational(contentFrames, 1).Mul(Rational{Num: 1, Den: 30})
	outputDuration := MustRational(12, 1).Mul(Rational{Num: 1, Den: 30})

	frames := make([]Frame, 0, 12)
	playerFrames := make([]PlayerFrame, 0, 12)
	for i := int64(0); i < 12; i++ {
		ticks := i
		frame := Frame{
			Index: i, PTSTicks: &ticks,
			TimeBase: &timeBase,
		}
		observed := PlayerFrame{Index: i, Presented: true}
		shownStart := MustRational(i, 1).Mul(Rational{Num: 1, Den: 30})
		observed.ShownStart = &shownStart
		if i < contentFrames {
			frame.Origin = "source"
			frame.SourceIndex = int64Ptr(i)
		} else {
			frame.Black = true
			frame.Origin = "tail_pad"
		}
		frames = append(frames, frame)
		playerFrames = append(playerFrames, observed)
	}

	return Evidence{
		PTSTimeBase: &timeBase,
		Output:      frames,
		Contract: Contract{
			FPS: fps, StartPTS: ZeroRational(), ContentFrames: contentFrames,
			DeliveryFrames: &contractFrames, DeliveryDuration: &contractDuration,
			AllowedTailBlackFrames: 0, TailPolicy: "reject_tail_black",
			ContainerDuration: &outputDuration, AudioDuration: &outputDuration,
			Player: PlayerEvidence{
				ReportedDuration: &outputDuration,
				Frames:           playerFrames,
			},
		},
	}
}
