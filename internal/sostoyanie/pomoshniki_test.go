package sostoyanie

import "time"

func timeSeychas() time.Time { return time.Now() }

func ptrStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.String()
	return &s
}
