package domain

import "time"

func ParseDate(value string) (time.Time, error) {
	date, err := time.Parse(time.DateOnly, value)
	if err != nil || date.Year() < 1 {
		return time.Time{}, Invalid("date must use YYYY-MM-DD")
	}
	return date, nil
}

func ValidateDates(start, end string) error {
	if _, err := ParseDate(start); err != nil {
		return err
	}
	if _, err := ParseDate(end); err != nil {
		return err
	}
	if end < start {
		return Invalid("end date precedes start date")
	}
	return nil
}
