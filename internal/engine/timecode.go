package engine

import "fmt"

func SecondsToTimecode(seconds float64) string {
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := int(seconds) % 60
	ms := int((seconds - float64(int(seconds))) * 1000)
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

func TimecodeToSeconds(tc string) (float64, error) {
	var h, m, s int
	var ms int
	_, err := fmt.Sscanf(tc, "%d:%d:%d.%d", &h, &m, &s, &ms)
	if err != nil {
		return 0, fmt.Errorf("invalid timecode: %s", tc)
	}
	return float64(h)*3600 + float64(m)*60 + float64(s) + float64(ms)/1000, nil
}
