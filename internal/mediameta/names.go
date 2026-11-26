package mediameta

func primariesName(value uint16) string {
	switch value {
	case 1:
		return "bt709"
	case 2:
		return "unspecified"
	case 4:
		return "bt470m"
	case 5:
		return "bt470bg"
	case 6:
		return "smpte170m"
	case 7:
		return "smpte240m"
	case 9:
		return "bt2020"
	case 11:
		return "display-p3"
	case 12:
		return "display-p3-p-d65"
	default:
		return "unknown"
	}
}

func transferName(value uint16) string {
	switch value {
	case 1:
		return "bt709"
	case 2:
		return "unspecified"
	case 4:
		return "gamma22"
	case 5:
		return "gamma28"
	case 6:
		return "smpte170m"
	case 7:
		return "smpte240m"
	case 13:
		return "iec61966-2-1"
	case 16:
		return "smpte2084"
	case 18:
		return "arib-std-b67"
	default:
		return "unknown"
	}
}

func matrixName(value uint16) string {
	switch value {
	case 0:
		return "gbr"
	case 1:
		return "bt709"
	case 2:
		return "unspecified"
	case 4:
		return "fcc"
	case 6:
		return "smpte170m"
	case 7:
		return "smpte240m"
	case 9:
		return "bt2020nc"
	case 10:
		return "bt2020c"
	default:
		return "unknown"
	}
}

func PixelFormatHasAlpha(pixFmt string) bool {
	switch pixFmt {
	case "rgba", "bgra", "argb", "abgr", "rgba64le", "rgba64be", "yuva420p",
		"yuva422p", "yuva444p", "yuva420p9le", "yuva420p10le", "yuva422p10le",
		"yuva444p9le", "yuva444p10le", "yuva444p12le", "yuva444p16le":
		return true
	}
	return len(pixFmt) >= 4 && pixFmt[:4] == "yuva"
}
