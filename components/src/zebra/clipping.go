package main

// BT.601 limited range; threshold against the nominal channel ceiling before clamping.
func clipped(y, u, v int) bool {
	yv := (y - 16) * 255 * 224 * 1000
	u -= 128
	v -= 128
	const white = 255 * 219 * 224 * 1000
	return yv+v*255*1402*219 >= white || yv-u*255*344*219-v*255*714*219 >= white || yv+u*255*1772*219 >= white
}

// Neutral plateau at LCD Y=198–199 in unassisted iLog (main-track Y'=796).
// RGB rails above still flag genuinely over-range display colours. The measured
// luma plateau is not a calibration of any individual sensor colour channel.
func warning(y, u, v int, log bool) bool { return (log && y >= 198) || clipped(y, u, v) }
