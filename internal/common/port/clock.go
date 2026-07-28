package port

import "time"

// SystemClock là Clock thật, dùng ở runtime. Test dùng mock/clock cố định.
type SystemClock struct{}

// Now trả về thời gian hiện tại theo UTC.
func (SystemClock) Now() time.Time { return time.Now().UTC() }
