package timestamp

import (
	"time"
	"fmt"
	"errors"
)


type OffsetManager struct {
    userOffsets map[int64]int
}

func NewOffsetManager() *OffsetManager {
    return &OffsetManager{
        userOffsets: make(map[int64]int),
    }
}

func (om *OffsetManager) SetUserOffset(userID int64, offset int) error {
    if offset < -12 || offset > 14 {
        return errors.New("смещение должно быть от -12 до +14 часов")
    }
    om.userOffsets[userID] = offset
    return nil
}

func (om *OffsetManager) GetUserOffset(userID int64) (int, bool) {
    offset, ok := om.userOffsets[userID]
    return offset, ok
}

func (om *OffsetManager) GetUserTime(userID int64) (string, error) {
    offset, ok := om.GetUserOffset(userID)
    if !ok {
        return "", errors.New("смещение не установлено")
    }
    
    loc := time.FixedZone("User", offset*3600)
	fmt.Println(loc, time.Now().In(loc).Format("15:04, 02.01.2006"), "weq")
    return time.Now().In(loc).Format("15:04, 02.01.2006"), nil
}

func OffsetToEmoji(offset int) string {
    if offset >= 0 {
        return fmt.Sprintf("UTC+%d", offset)
    }
    return fmt.Sprintf("UTC%d", offset) // минус сам отобразится
}