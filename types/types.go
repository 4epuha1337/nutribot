package types

type TimeEntry struct {
    Hour   int `json:"hour"`
    Minute int `json:"minute"`
}

type User struct{
    Id int `json:"id"`
    ChatID int `json:"chatid"`
    Offset int `json:"offset"`
    Time []TimeEntry `json:"time"`
    Name string `json:"name"`
    Age int `json:"age"`
    State int
}

type ReminderWithID struct {
    ID int64
    Reminder TimeEntry
}