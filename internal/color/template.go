package color

type RGB struct {
	R, G, B float64
}

type BarsTemplate struct {
	Name   string
	Level  float64
	Colors [7]RGB
}

// Top-row color bars, in the conventional left-to-right order.
var Templates = []BarsTemplate{
	{Name: "75%", Level: 0.75, Colors: barColors(0.75)},
	{Name: "100%", Level: 1.00, Colors: barColors(1.00)},
}

func barColors(level float64) [7]RGB {
	return [7]RGB{
		{R: level, G: level, B: level}, // white
		{R: level, G: level, B: 0},     // yellow
		{R: 0, G: level, B: level},     // cyan
		{R: 0, G: level, B: 0},         // green
		{R: level, G: 0, B: level},     // magenta
		{R: level, G: 0, B: 0},         // red
		{R: 0, G: 0, B: level},         // blue
	}
}

func ResidualRGB(a, b RGB) float64 {
	dr, dg, db := a.R-b.R, a.G-b.G, a.B-b.B
	return (dr*dr + dg*dg + db*db) / 3
}
