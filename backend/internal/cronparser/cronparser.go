package cronparser

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type FieldType int

const (
	FieldSecond FieldType = iota
	FieldMinute
	FieldHour
	FieldDayOfMonth
	FieldMonth
	FieldDayOfWeek
	FieldYear
)

var fieldNames = map[FieldType]string{
	FieldSecond:     "秒",
	FieldMinute:     "分",
	FieldHour:       "时",
	FieldDayOfMonth: "日",
	FieldMonth:      "月",
	FieldDayOfWeek:  "周",
	FieldYear:       "年",
}

type FieldRange struct {
	Min, Max int
}

var fieldRanges = map[FieldType]FieldRange{
	FieldSecond:     {0, 59},
	FieldMinute:     {0, 59},
	FieldHour:       {0, 23},
	FieldDayOfMonth: {1, 31},
	FieldMonth:      {1, 12},
	FieldDayOfWeek:  {0, 6},
	FieldYear:       {1970, 2099},
}

type FieldValue struct {
	Values      map[int]bool
	HasLast     bool
	HasLastWeek  bool
	NearestWeekday int
	NthWeekday  int
	NthWeek     int
}

func NewFieldValue() *FieldValue {
	return &FieldValue{Values: make(map[int]bool)}
}

type CronExpression struct {
	Fields     []*FieldValue
	FieldTypes []FieldType
	IsExtended bool
	Raw        string
}

type ParseError struct {
	Field    string
	Position int
	Message  string
}

func (e *ParseError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("字段[%s]语法错误: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("语法错误(位置%d): %s", e.Position, e.Message)
}

func Parse(expr string) (*CronExpression, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, &ParseError{Message: "表达式不能为空"}
	}

	fields := strings.Fields(expr)
	var fieldTypes []FieldType
	var isExtended bool

	switch len(fields) {
	case 5:
		fieldTypes = []FieldType{FieldMinute, FieldHour, FieldDayOfMonth, FieldMonth, FieldDayOfWeek}
		isExtended = false
	case 7:
		fieldTypes = []FieldType{FieldSecond, FieldMinute, FieldHour, FieldDayOfMonth, FieldMonth, FieldDayOfWeek, FieldYear}
		isExtended = true
	default:
		return nil, &ParseError{Message: fmt.Sprintf("字段数量错误: 需要5个或7个字段,实际%d个", len(fields))}
	}

	cron := &CronExpression{
		Fields:     make([]*FieldValue, len(fieldTypes)),
		FieldTypes: fieldTypes,
		IsExtended: isExtended,
		Raw:        expr,
	}

	for i, field := range fields {
		ft := fieldTypes[i]
		fv, err := parseField(field, ft)
		if err != nil {
			return nil, err
		}
		cron.Fields[i] = fv
	}

	return cron, nil
}

func parseField(field string, ft FieldType) (*FieldValue, error) {
	rng := fieldRanges[ft]
	fv := NewFieldValue()
	name := fieldNames[ft]

	if field == "" {
		return nil, &ParseError{Field: name, Message: "字段不能为空"}
	}

	if ft == FieldDayOfMonth {
		if field == "L" {
			fv.HasLast = true
			return fv, nil
		}
		if strings.HasSuffix(field, "W") {
			dayStr := strings.TrimSuffix(field, "W")
			day, err := strconv.Atoi(dayStr)
			if err != nil {
				return nil, &ParseError{Field: name, Message: fmt.Sprintf("无效的工作日语法: %s", field)}
			}
			if day < 1 || day > 31 {
				return nil, &ParseError{Field: name, Message: fmt.Sprintf("日期超出范围: %d", day)}
			}
			fv.NearestWeekday = day
			return fv, nil
		}
		if strings.HasSuffix(field, "L") && len(field) > 1 {
			prefix := strings.TrimSuffix(field, "L")
			weekday, err := strconv.Atoi(prefix)
			if err != nil {
				return nil, &ParseError{Field: name, Message: fmt.Sprintf("无效的月末语法: %s", field)}
			}
			if weekday < 1 || weekday > 7 {
				return nil, &ParseError{Field: name, Message: fmt.Sprintf("星期超出范围: %d", weekday)}
			}
			fv.HasLastWeek = true
			fv.NthWeekday = weekday % 7
			return fv, nil
		}
	}

	if ft == FieldDayOfWeek && strings.Contains(field, "#") {
		parts := strings.Split(field, "#")
		if len(parts) != 2 {
			return nil, &ParseError{Field: name, Message: fmt.Sprintf("无效的第N个星期语法: %s", field)}
		}
		weekday, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, &ParseError{Field: name, Message: fmt.Sprintf("无效的星期值: %s", parts[0])}
		}
		if weekday < 1 || weekday > 7 {
			return nil, &ParseError{Field: name, Message: fmt.Sprintf("星期超出范围: %d", weekday)}
		}
		nth, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, &ParseError{Field: name, Message: fmt.Sprintf("无效的序号: %s", parts[1])}
		}
		if nth < 1 || nth > 5 {
			return nil, &ParseError{Field: name, Message: fmt.Sprintf("序号超出范围: %d", nth)}
		}
		fv.NthWeekday = weekday % 7
		fv.NthWeek = nth
		return fv, nil
	}

	if ft == FieldDayOfWeek && strings.HasSuffix(field, "L") {
		dayStr := strings.TrimSuffix(field, "L")
		weekday, err := strconv.Atoi(dayStr)
		if err != nil {
			return nil, &ParseError{Field: name, Message: fmt.Sprintf("无效的周末最后一天语法: %s", field)}
		}
		if weekday < 1 || weekday > 7 {
			return nil, &ParseError{Field: name, Message: fmt.Sprintf("星期超出范围: %d", weekday)}
		}
		fv.HasLastWeek = true
		fv.NthWeekday = weekday % 7
		return fv, nil
	}

	parts := strings.Split(field, ",")
	for _, part := range parts {
		if err := parsePart(part, ft, fv); err != nil {
			return nil, err
		}
	}

	for v := range fv.Values {
		if v < rng.Min || v > rng.Max {
			return nil, &ParseError{Field: name, Message: fmt.Sprintf("值超出范围[%d-%d]: %d", rng.Min, rng.Max, v)}
		}
	}

	return fv, nil
}

func parsePart(part string, ft FieldType, fv *FieldValue) error {
	name := fieldNames[ft]
	rng := fieldRanges[ft]

	if part == "" {
		return &ParseError{Field: name, Message: "子表达式不能为空"}
	}

	step := 1
	var rangeStr string

	if strings.Contains(part, "/") {
		slashParts := strings.SplitN(part, "/", 2)
		rangeStr = slashParts[0]
		if len(slashParts) != 2 || slashParts[1] == "" {
			return &ParseError{Field: name, Message: fmt.Sprintf("无效的间隔语法: %s", part)}
		}
		var err error
		step, err = strconv.Atoi(slashParts[1])
		if err != nil {
			return &ParseError{Field: name, Message: fmt.Sprintf("无效的间隔值: %s", slashParts[1])}
		}
		if step <= 0 {
			return &ParseError{Field: name, Message: "间隔值必须为正整数"}
		}
	} else {
		rangeStr = part
	}

	var start, end int
	if rangeStr == "*" || rangeStr == "?" {
		start = rng.Min
		end = rng.Max
	} else if strings.Contains(rangeStr, "-") {
		dashParts := strings.SplitN(rangeStr, "-", 2)
		var err error
		start, err = strconv.Atoi(dashParts[0])
		if err != nil {
			return &ParseError{Field: name, Message: fmt.Sprintf("无效的范围起始值: %s", dashParts[0])}
		}
		end, err = strconv.Atoi(dashParts[1])
		if err != nil {
			return &ParseError{Field: name, Message: fmt.Sprintf("无效的范围结束值: %s", dashParts[1])}
		}
	} else {
		val, err := strconv.Atoi(rangeStr)
		if err != nil {
			return &ParseError{Field: name, Message: fmt.Sprintf("无效的值: %s", rangeStr)}
		}
		if step == 1 {
			fv.Values[val] = true
			return nil
		}
		start = val
		end = rng.Max
	}

	for v := start; v <= end; v += step {
		fv.Values[v] = true
	}

	return nil
}

func (c *CronExpression) NextN(from time.Time, n int) []time.Time {
	if n <= 0 {
		return []time.Time{}
	}

	results := make([]time.Time, 0, n)
	current := from.Truncate(time.Second).Add(time.Second)
	loc := from.Location()
	maxIterations := 366 * 24 * 60 * 60
	iterations := 0

	for len(results) < n && iterations < maxIterations {
		iterations++
		if next, ok := c.findNext(current, loc); ok {
			results = append(results, next)
			current = next.Add(time.Second)
		} else {
			break
		}
	}

	return results
}

func (c *CronExpression) findNext(from time.Time, loc *time.Location) (time.Time, bool) {
	t := from
	for i := 0; i < 366*24*60*60; i++ {
		if c.matches(t) {
			return t, true
		}
		t = t.Add(time.Second)
	}
	return time.Time{}, false
}

func (c *CronExpression) HasTriggerBetween(start, end time.Time) bool {
	if !start.Before(end) {
		return false
	}
	next := c.NextN(start, 1)
	if len(next) == 0 {
		return false
	}
	return next[0].Before(end) || next[0].Equal(end)
}

func (c *CronExpression) matches(t time.Time) bool {
	second := t.Second()
	minute := t.Minute()
	hour := t.Hour()
	_ = t.Day()
	month := int(t.Month())
	weekday := int(t.Weekday())
	year := t.Year()

	idx := 0
	if c.IsExtended {
		if !c.Fields[idx].Values[second] {
			return false
		}
		idx++
	}
	if !c.Fields[idx].Values[minute] {
		return false
	}
	idx++
	if !c.Fields[idx].Values[hour] {
		return false
	}
	idx++

	domField := c.Fields[idx]
	idx++
	monField := c.Fields[idx]
	idx++
	dowField := c.Fields[idx]

	if !monField.Values[month] {
		return false
	}

	if !c.matchDayOfMonth(domField, t) {
		return false
	}
	if !c.matchDayOfWeek(dowField, t) {
		return false
	}

	if c.IsExtended {
		idx++
		if !c.Fields[idx].Values[year] {
			return false
		}
	}

	_ = weekday
	return true
}

func (c *CronExpression) matchDayOfMonth(f *FieldValue, t time.Time) bool {
	if f.HasLast {
		return t.Day() == lastDayOfMonth(t)
	}
	if f.NearestWeekday > 0 {
		return t.Day() == nearestWeekday(t, f.NearestWeekday)
	}
	if f.HasLastWeek {
		return isLastWeekdayOfMonth(t, f.NthWeekday)
	}
	return f.Values[t.Day()]
}

func (c *CronExpression) matchDayOfWeek(f *FieldValue, t time.Time) bool {
	if f.NthWeek > 0 {
		return isNthWeekdayOfMonth(t, f.NthWeekday, f.NthWeek)
	}
	if f.HasLastWeek {
		return isLastWeekdayOfMonth(t, f.NthWeekday)
	}
	if len(f.Values) == 0 {
		return true
	}
	return f.Values[int(t.Weekday())]
}

func lastDayOfMonth(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
}

func nearestWeekday(t time.Time, target int) int {
	year, month, _ := t.Date()
	targetDate := time.Date(year, month, target, 0, 0, 0, 0, t.Location())
	wd := targetDate.Weekday()
	switch wd {
	case time.Saturday:
		if target > 1 {
			return target - 1
		}
		return target + 2
	case time.Sunday:
		ld := lastDayOfMonth(t)
		if target < ld {
			return target + 1
		}
		return target - 2
	default:
		return target
	}
}

func isNthWeekdayOfMonth(t time.Time, weekday, nth int) bool {
	if int(t.Weekday()) != weekday {
		return false
	}
	return (t.Day()-1)/7+1 == nth
}

func isLastWeekdayOfMonth(t time.Time, weekday int) bool {
	if int(t.Weekday()) != weekday {
		return false
	}
	return t.Day()+7 > lastDayOfMonth(t)
}

func Validate(expr string) error {
	_, err := Parse(expr)
	return err
}

func PreviewNext(expr string, n int) ([]time.Time, error) {
	c, err := Parse(expr)
	if err != nil {
		return nil, err
	}
	return c.NextN(time.Now(), n), nil
}

func (c *CronExpression) Describe() string {
	parts := make([]string, len(c.FieldTypes))
	for i, ft := range c.FieldTypes {
		f := c.Fields[i]
		var desc string
		if f.HasLast {
			desc = "最后一天"
		} else if f.NearestWeekday > 0 {
			desc = fmt.Sprintf("最近工作日(%d)", f.NearestWeekday)
		} else if f.HasLastWeek {
			desc = fmt.Sprintf("最后一个星期%d", f.NthWeekday)
		} else if f.NthWeek > 0 {
			desc = fmt.Sprintf("第%d个星期%d", f.NthWeek, f.NthWeekday)
		} else if len(f.Values) == fieldRanges[ft].Max-fieldRanges[ft].Min+1 {
			desc = "每" + fieldNames[ft]
		} else {
			desc = valuesToString(f.Values)
		}
		parts[i] = fieldNames[ft] + ":" + desc
	}
	return strings.Join(parts, " ")
}

func valuesToString(m map[int]bool) string {
	if len(m) == 0 {
		return "-"
	}
	vals := make([]int, 0, len(m))
	for v := range m {
		vals = append(vals, v)
	}
	for i := range vals {
		for j := i + 1; j < len(vals); j++ {
			if vals[i] > vals[j] {
				vals[i], vals[j] = vals[j], vals[i]
			}
		}
	}
	strs := make([]string, 0, len(vals))
	i := 0
	for i < len(vals) {
		start := vals[i]
		end := start
		for i+1 < len(vals) && vals[i+1] == end+1 {
			end = vals[i+1]
			i++
		}
		if start == end {
			strs = append(strs, strconv.Itoa(start))
		} else {
			strs = append(strs, fmt.Sprintf("%d-%d", start, end))
		}
		i++
	}
	return strings.Join(strs, ",")
}

var _ = math.MaxInt32
