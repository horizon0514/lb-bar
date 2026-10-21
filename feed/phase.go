package feed

import (
	"fmt"
	"time"
	_ "time/tzdata" // America/New_York, wherever the app runs
)

var newYork = func() *time.Location {
	l, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.FixedZone("EST", -5*3600)
	}
	return l
}()

type span struct {
	from, to int // minutes of the day, local to the market
	phase    Phase
}

var schedules = map[string][]span{
	"HK": {{9 * 60, 9*60 + 30, PreOpen}, {9*60 + 30, 12 * 60, Trading}, {12 * 60, 13 * 60, Break}, {13 * 60, 16 * 60, Trading}, {16 * 60, 16*60 + 10, PostClose}},
	"CN": {{9*60 + 15, 9*60 + 30, PreOpen}, {9*60 + 30, 11*60 + 30, Trading}, {11*60 + 30, 13 * 60, Break}, {13 * 60, 15 * 60, Trading}},
	"US": {{4 * 60, 9*60 + 30, PreOpen}, {9*60 + 30, 16 * 60, Trading}, {16 * 60, 20 * 60, PostClose}},
}

// ScheduledPhase is the phase a market's timetable has at t, holidays not
// known: what the app shows until, or when, the API's status can't be had.
func ScheduledPhase(market string, t time.Time) Phase {
	loc := Shanghai
	if market == "US" {
		loc = newYork
	}
	l := t.In(loc)
	m := l.Hour()*60 + l.Minute()
	wd := l.Weekday()
	if market == "US" {
		// Overnight trading: Sunday to Thursday, 20:00 to 04:00 the next day.
		if (m >= 20*60 && wd >= time.Sunday && wd <= time.Thursday) || (m < 4*60 && wd >= time.Monday && wd <= time.Friday) {
			return Night
		}
	}
	if wd == time.Saturday || wd == time.Sunday {
		return Closed
	}
	for _, s := range schedules[market] {
		if m >= s.from && m < s.to {
			return s.phase
		}
	}
	return Closed
}

// Hours are a market's regular trading hours, in Beijing time, on the
// day of t: US hours move with daylight saving time.
func Hours(market string, t time.Time) string {
	switch market {
	case "HK":
		return "09:30–12:00, 13:00–16:00"
	case "CN":
		return "09:30–11:30, 13:00–15:00"
	case "US":
		ny := t.In(newYork)
		open := time.Date(ny.Year(), ny.Month(), ny.Day(), 9, 30, 0, 0, newYork).In(Shanghai)
		close := time.Date(ny.Year(), ny.Month(), ny.Day(), 16, 0, 0, 0, newYork).In(Shanghai)
		pre := time.Date(ny.Year(), ny.Month(), ny.Day(), 4, 0, 0, 0, newYork).In(Shanghai)
		post := time.Date(ny.Year(), ny.Month(), ny.Day(), 20, 0, 0, 0, newYork).In(Shanghai)
		return fmt.Sprintf("盘前 %s，盘中 %s–%s，盘后至 %s", pre.Format("15:04"), open.Format("15:04"), close.Format("15:04"), post.Format("15:04"))
	}
	return ""
}

// Label says a phase in Chinese.
func (p Phase) Label(market string) string {
	switch p {
	case Trading:
		return "交易中"
	case PreOpen:
		if market == "US" {
			return "盘前"
		}
		return "竞价"
	case PostClose:
		if market == "US" {
			return "盘后"
		}
		return "收市竞价"
	case Break:
		return "午休"
	case Night:
		return "夜盘"
	}
	return "已收盘"
}

// MarketName is a market's name in Chinese.
func MarketName(m string) string {
	switch m {
	case "HK":
		return "港股"
	case "US":
		return "美股"
	case "CN":
		return "A股"
	case "SG":
		return "新加坡"
	}
	return m
}

// SessionMinutes is how many minutes a market's regular day has: the
// intraday chart's width.
func SessionMinutes(market string) int {
	switch market {
	case "HK":
		return 330
	case "CN":
		return 240
	case "US":
		return 390
	}
	return 0
}
