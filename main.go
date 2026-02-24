package main

import (
	"encoding/json"
	"fmt"
	"log"
	rd "nutribot/reminder"
	ts "nutribot/timestamp"
	"nutribot/types"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "github.com/joho/godotenv"
)

var  (
    Users []types.User
    OffsetManager *ts.OffsetManager
    UsersMutex sync.RWMutex
    ReminderScheduler *rd.Scheduler
)
func InitUsers () error{
    OffsetManager = ts.NewOffsetManager()
    data, err := os.ReadFile("users.json")
    if err == nil {
        err = json.Unmarshal(data, &Users)
        if err == nil {
            fmt.Println("Список пользователей успешно загружен")
            loadedCount := 0
            for _, user := range Users {
                err := OffsetManager.SetUserOffset(int64(user.Id), user.Offset)
                if err != nil {
                    fmt.Printf("Ошибка при установке offset %d для пользователя %d: %v\n", 
                        user.Offset, user.Id, err)
                } else {
                    loadedCount++
                }
            }
            
            fmt.Printf("Загружено %d пользователей. Offset установлен для %d из них\n", 
                len(Users), loadedCount)
            return nil
        }
    }

    fmt.Println("Файл не найден. Создаю новый...")
    file, err := os.Create("users.json")
    if err != nil {
        fmt.Println("Ошибка при содании файла: ", err)
        return err
    }
    var nullFile []byte
    nullFile = append(nullFile, '[', ']')
    _, err = file.Write(nullFile)
    if err != nil {
        fmt.Println("Ошибка при инициировании нового файла: ", err)
        return err
    }
    return nil
}

func UserExist(id int) (int, bool) {
    for i, u := range Users {
        if id == u.Id {
            return i, true
        } 
    }
    return -1, false
}

func hasWhitespace(s string) bool {
    return strings.ContainsFunc(s, unicode.IsSpace)
}

func ParseTimeString(timeStr string) ([]types.TimeEntry, error) {
    timeStr = strings.ReplaceAll(timeStr, " ", "")
    
    if timeStr == "" {
        return nil, fmt.Errorf("строка пустая")
    }
    
    parts := strings.Split(timeStr, ",")
    
    if len(parts) == 0 {
        return nil, fmt.Errorf("не указано ни одного времени")
    }
    
    if len(parts) > 10 {
        return nil, fmt.Errorf("слишком много значений времени (максимум 10)")
    }
    
    times := make([]types.TimeEntry, 0, len(parts))
    seen := make(map[string]bool)
    
    for i, part := range parts {
        if !isValidTimeFormat(part) {
            return nil, fmt.Errorf("неверный формат времени в %d-м значении: %s (нужно ЧЧ:ММ)", i+1, part)
        }
        
        timeParts := strings.Split(part, ":")
        hours, _ := strconv.Atoi(timeParts[0])
        minutes, _ := strconv.Atoi(timeParts[1])
        
        if hours < 0 || hours > 23 {
            return nil, fmt.Errorf("часы должны быть от 0 до 23 в значении %s", part)
        }
        if minutes < 0 || minutes > 59 {
            return nil, fmt.Errorf("минуты должны быть от 0 до 59 в значении %s", part)
        }
        
        key := fmt.Sprintf("%02d:%02d", hours, minutes)
        if seen[key] {
            return nil, fmt.Errorf("обнаружен повтор времени %s", key)
        }
        seen[key] = true
        
        times = append(times, types.TimeEntry{
            Hour:   hours,
            Minute: minutes,
        })
    }
    
    return times, nil
}

func isValidTimeFormat(s string) bool {
    if len(s) != 5 || s[2] != ':' {
        return false
    }
    
    for i := 0; i < 5; i++ {
        if i == 2 {
            continue
        }
        if s[i] < '0' || s[i] > '9' {
            return false
        }
    }
    
    return true
}

func FormatTimeEntry(t types.TimeEntry) string {
    return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
}

func TimeEntryToMinutes(t types.TimeEntry) int {
    return t.Hour*60 + t.Minute
}

func MinutesToTimeEntry(minutes int) types.TimeEntry {
    return types.TimeEntry{
        Hour:   minutes / 60,
        Minute: minutes % 60,
    }
}

func SaveUsers() error {
    if _, err := os.Stat("users.json"); err == nil {
        backupName := fmt.Sprintf("users_backup_%s.json", 
            time.Now().Format("2006-01-02_15-04-05"))
        
        input, err := os.ReadFile("users.json")
        if err == nil {
            os.WriteFile(backupName, input, 0644)
            cleanupOldBackups()
        }
    }

    data, err := json.MarshalIndent(Users, "", "  ")
    if err != nil {
        log.Printf("Ошибка при маршалинге пользователей: %v", err)
        return fmt.Errorf("ошибка преобразования данных: %v", err)
    }

    err = os.WriteFile("users.json", data, 0644)
    if err != nil {
        log.Printf("Ошибка при записи в файл users.json: %v", err)
        return fmt.Errorf("не удалось записать файл: %v", err)
    }

    log.Printf("Пользователи успешно сохранены. Всего: %d", len(Users))
    return nil
}

func cleanupOldBackups() {
    files, err := filepath.Glob("users_backup_*.json")
    if err != nil {
        return
    }

    if len(files) > 5 {
        for i := 0; i < len(files)-5; i++ {
            os.Remove(files[i])
        }
    }
}

func showMainMenu(u types.User) tgbotapi.ReplyKeyboardMarkup{
    keyboard := tgbotapi.NewReplyKeyboard(
        tgbotapi.NewKeyboardButtonRow(
            tgbotapi.NewKeyboardButton("Профиль"),
            tgbotapi.NewKeyboardButton("Напоминания"),
        ),
    )
    if u.State == -1 {
        keyboard.Keyboard = append(keyboard.Keyboard, tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("Включить уведомления")))
    } else {
        keyboard.Keyboard = append(keyboard.Keyboard, tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("Отключить уведомления")))
    }

    keyboard.OneTimeKeyboard = false
    return keyboard
}

func showNotificationsMenu() tgbotapi.ReplyKeyboardMarkup{
    keyboard := tgbotapi.NewReplyKeyboard(
        tgbotapi.NewKeyboardButtonRow(
            tgbotapi.NewKeyboardButton("Добавить"),
            tgbotapi.NewKeyboardButton("Удалить"),
        ),
        tgbotapi.NewKeyboardButtonRow(
            tgbotapi.NewKeyboardButton("Назад"),
        ),
    )

    keyboard.OneTimeKeyboard = false
    return keyboard
}

func timeToStr(t types.TimeEntry) string {
    return fmt.Sprintf("%d:%d", t.Hour, t.Minute)
}

func showDeleteMenu(u types.User) tgbotapi.InlineKeyboardMarkup{
    keyboard := tgbotapi.NewInlineKeyboardMarkup()
    var row []tgbotapi.InlineKeyboardButton
    for i, t := range u.Time {
        if i % 2 == 0 {
            row = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(timeToStr(t), fmt.Sprintf("%d", i+1)))
        } else {
            row = append(row, tgbotapi.NewInlineKeyboardButtonData(timeToStr(t), fmt.Sprintf("%d", i+1)))
            keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, row)
        }
    }
    if len(row) == 1 {
        row = append(row, tgbotapi.NewInlineKeyboardButtonData("Отмена", "cancel"))
        keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, row) 
    } else {
        row = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Отмена", "cancel"))
        keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, row) 
    }
    return keyboard
}

func addAllTimeToText(u types.User, text string) string {
    text = text + " Текущие уведомления:\n"
    for j, time := range u.Time {
        text = fmt.Sprintf("%s\n%d - %d:%d", text, j + 1, time.Hour, time.Minute)
    }
    return text
}

func handleCallback(callback *tgbotapi.CallbackQuery, bot *tgbotapi.BotAPI) {
    bot.Request(tgbotapi.NewCallback(callback.ID, ""))

    var msg tgbotapi.MessageConfig

    data := callback.Data
    chatID := callback.Message.Chat.ID
    messageID := callback.Message.MessageID
    userID := callback.From.ID

    fmt.Printf("Callback получен: data=%s, userID=%d\n", data, userID)

    UsersMutex.Lock()
    defer UsersMutex.Unlock()

    i, flag := UserExist(int(userID))
    if !flag {
        bot.Send(tgbotapi.NewMessage(chatID, "Привет, для начала нужно зарегистрироваться. Для этого напиши /start"))
        return
    }

    switch data {
    case "cancel":
        msg = tgbotapi.NewMessage(chatID, "Удаление напоминания отменено.")
        msg.ReplyMarkup = showNotificationsMenu()
        bot.Send(msg)

        Users[i].State = 0

        delMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
        bot.Request(delMsg)
        return
    default:
        index, err := strconv.Atoi(data)
        if err != nil {
            return
        }
        if index < 1 || index > len(Users[i].Time) {
            msg = tgbotapi.NewMessage(chatID, fmt.Sprintf(`Данного напоминания не существует, проверьте еще раз.`))
            bot.Send(msg)
            return
        }
        times := Users[i].Time[index-1]
        Users[i].Time = append(Users[i].Time[:index-1], Users[i].Time[index:]...)
        Users[i].State = 0
        if err = SaveUsers(); err != nil {
            fmt.Printf("Ошибка сохранения пользователя: %v", err)
            return
        }
        text := fmt.Sprintf(`Уведомление на %d:%d успешно удалено.`, times.Hour, times.Minute)
        if len(Users[i].Time) > 0 {
            text = addAllTimeToText(Users[i], text)
        } else {
            text = text + " У вас нет установленных уведомлений"
        }
        msg = tgbotapi.NewMessage(chatID, text)
        msg.ReplyMarkup = showNotificationsMenu()
        delMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
        bot.Request(delMsg)
        bot.Send(msg)
        return
    }
}

func main() {
    _ = godotenv.Load()
    err := InitUsers()
    if err != nil {
        fmt.Println("Ошибка: ", err)
        return
    }
    token := os.Getenv("BOT_TOKEN")
    bot, err := tgbotapi.NewBotAPI(token)
    if err != nil {
        log.Panic(err)
    }

    ReminderScheduler = rd.NewScheduler(
        bot,
        &Users,
        &UsersMutex,
        OffsetManager,
    )

    ReminderScheduler.Start()
    defer ReminderScheduler.Stop()
    u := tgbotapi.NewUpdate(0)
    u.Timeout = 60

    updates := bot.GetUpdatesChan(u)
    for update := range updates {
        if update.CallbackQuery != nil {
                handleCallback(update.CallbackQuery, bot)
                continue
            }

        if update.Message != nil {
            log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

            UsersMutex.Lock()
            
            var msg tgbotapi.MessageConfig

            if update.Message.IsCommand() {
                switch update.Message.Command() {
                case "start":
                    if i, flag := UserExist(int(update.Message.From.ID)); flag {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Привет, %s! Чтобы посмотреть свои уведомления, напиши /list. Для добавления нового уведомления напиши /reminder, для удаления - /delete. Для отмены отправки уведомлений напиши /cancel", Users[i].Name))
                        msg.ReplyMarkup = showMainMenu(Users[i])
                        bot.Send(msg)
                        UsersMutex.Unlock()
                        continue
                    }
                    var newUser types.User
                    newUser.Id = int(update.Message.From.ID)
                    newUser.ChatID = int(update.Message.Chat.ID)
                    newUser.State = 1
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, `Привет! Давай познакомимся. Меня зовут Морковка, я буду твоим <ВСТАВИТЬ ТЕКСТ>.
Для начала, давай познакомимся. Как тебя зовут?`)
                    Users = append(Users, newUser)
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                case "reminder":
                    i, flag := UserExist(int(update.Message.From.ID))
                    if !flag {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Привет, для начала нужно зарегистрироваться. Для этого напиши /start"))
                        bot.Send(msg)
                        UsersMutex.Unlock()
                        continue
                    }
                    if len(Users[i].Time) > 10 {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("У тебя уже есть 10 уведомлений."))
                        bot.Send(msg)
                        UsersMutex.Unlock()
                        continue
                    }
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Для добавления нового уведомления, напиши время нового уведомления в формате hh:mm. Для отмены, напиши cancel"))
                    msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
                    bot.Send(msg)
                    Users[i].State = 5
                    UsersMutex.Unlock()
                    continue
                case "delete":
                    i, flag := UserExist(int(update.Message.From.ID))
                    if !flag {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Привет, для начала нужно зарегистрироваться. Для этого напиши /start"))
                        bot.Send(msg)
                        UsersMutex.Unlock()
                        continue
                    }
                    if len(Users[i].Time) == 0 {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("У тебя уже нет уведомлений."))
                        bot.Send(msg)
                        UsersMutex.Unlock()
                        continue
                    }
                    text := fmt.Sprintf(`Для удаления уведомления, напиши номер удаляемого уведомления, либо нажми на кнопку. Установленные уведомления:`)
                    for j, time := range Users[i].Time {
                        text = fmt.Sprintf("%s\n%d - %d:%d", text, j+1, time.Hour, time.Minute)
                    }
                    text = text + "\nДля отмены, напиши cancel"
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                    msg.ReplyMarkup = showDeleteMenu(Users[i])
                    bot.Send(msg)
                    Users[i].State = 6
                    UsersMutex.Unlock()
                    continue
                case "cancel":
                    i, flag := UserExist(int(update.Message.From.ID))
                    if !flag {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Привет, для начала нужно зарегистрироваться. Для этого напиши /start"))
                        bot.Send(msg)
                        UsersMutex.Unlock()
                        continue
                    }
                    if Users[i].State == -1 {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Отправка уведомлений снова включена!"))
                        msg.ReplyMarkup = showMainMenu(Users[i])
                        bot.Send(msg)
                        Users[i].State = 0
                        if err = SaveUsers(); err != nil {
                            fmt.Printf("Ошибка сохранения пользователя: %v", err)
                            UsersMutex.Unlock()
                            continue
                        }
                        UsersMutex.Unlock()
                        continue
                    }
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Отправка уведомлений отключена."))
                    msg.ReplyMarkup = showMainMenu(Users[i])
                    bot.Send(msg)
                    Users[i].State = -1
                    if err = SaveUsers(); err != nil {
                        fmt.Printf("Ошибка сохранения пользователя: %v", err)
                        UsersMutex.Unlock()
                        continue
                    }
                    UsersMutex.Unlock()
                    continue
                case "list":
                    i, flag := UserExist(int(update.Message.From.ID))
                    if !flag {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Привет, для начала нужно зарегистрироваться. Для этого напиши /start"))
                        bot.Send(msg)
                        UsersMutex.Unlock()
                        continue
                    }
                    if len(Users[i].Time) == 0 {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("У вас нет установленных уведомлений."))
                        bot.Send(msg)
                        UsersMutex.Unlock()
                        continue
                    }
                    text := fmt.Sprintf(`Установленные уведомления:`)
                    for j, time := range Users[i].Time {
                        text = fmt.Sprintf("%s\n%d - %d:%d", text, j+1, time.Hour, time.Minute)
                    }
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                    msg.ReplyMarkup = showNotificationsMenu()
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                default:
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Извини, не знаю такой команды")
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
            }

            i, flag := UserExist(int(update.Message.From.ID))
            if !flag {
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Привет! Для того, чтобы пользоваться мной, необходимо зарегистрироваться. напиши /start")
                bot.Send(msg)
                UsersMutex.Unlock()
                continue
            }

            switch Users[i].State {
            case 1:
                if hasWhitespace(update.Message.Text) {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Напиши только имя")
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
                Users[i].Name = update.Message.Text
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Приятно познакомится, %s! А сколько тебе лет?", Users[i].Name))
                bot.Send(msg)
                Users[i].State = 2
                UsersMutex.Unlock()
                continue
            case 2:
                age, err := strconv.Atoi(update.Message.Text)
                if err != nil {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Это не похоже на возраст, попробуй еще раз!")
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
                Users[i].Age = age
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Для корректной работы также нужно узнать ваш часовой пояс. Введите только сдвиг относительно UTC0 (Для Москвы, Санкт-Петербурга - 3)"))
                bot.Send(msg)
                Users[i].State = 3
                UsersMutex.Unlock()
                continue
            case 3:
                offset, err := strconv.Atoi(update.Message.Text)
                if err != nil {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Это не похоже на сдвиг, попробуй еще раз!")
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
                if offset > 14 || offset < -12 {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Это неверный сдвиг, он должен быть в промежутке от -12 до 14, попробуй еще раз!")
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
                Users[i].Offset = offset
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Когда тебе напоминать о приеме пищи? Напиши время в формате hh:mm, несколько раз через запятую, но не более 10"))
                bot.Send(msg)
                Users[i].State = 4
                UsersMutex.Unlock()
                continue
            case 4:
                times, err := ParseTimeString(update.Message.Text)
                if err != nil {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf(`Пожалуйста, укажите время в формате hh:mm, hh:mm
Например: 19:20, 15:40`))
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
                Users[i].Time = times
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Отлично! Я все запомнил, и теперь буду напоминать тебе о необходимости приема пищи!"))
                msg.ReplyMarkup = showMainMenu(Users[i])
                bot.Send(msg)
                Users[i].State = 0
                err = OffsetManager.SetUserOffset(int64(Users[i].ChatID), Users[i].Offset)
                if err != nil {
                    fmt.Printf("Ошибка установки offset: %v\n", err)
                }
                if err = SaveUsers(); err != nil {
                    fmt.Printf("Ошибка сохранения пользователя: %v", err)
                    UsersMutex.Unlock()
                    continue
                }
                UsersMutex.Unlock()
                continue
            case 5:
                if update.Message.Text == "cancel" {
                    text := fmt.Sprintf("Добавление нового напоминания отменено.")
                    if len(Users[i].Time) > 0 {
                        text = addAllTimeToText(Users[i], text)
                    } else {
                        text = text + " У вас нет установленных уведомлений"
                    }
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                    msg.ReplyMarkup = showNotificationsMenu()
                    bot.Send(msg)
                    Users[i].State = 0
                    UsersMutex.Unlock()
                    continue
                }
                times, err := ParseTimeString(update.Message.Text)
                if err != nil {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf(`Пожалуйста, укажите время в формате hh:mm, hh:mm
Например: 19:20, 15:40`))
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
                Users[i].Time = append(Users[i].Time, times...)
                Users[i].State = 0
                if err = SaveUsers(); err != nil {
                    fmt.Printf("Ошибка сохранения пользователя: %v", err)
                    UsersMutex.Unlock()
                    continue
                }
                text := fmt.Sprintf("Новое уведомление на %d:%d успешно установлено.", times[0].Hour, times[0].Minute)
                if len(Users[i].Time) > 0 {
                    text = addAllTimeToText(Users[i], text)
                } else {
                    text = text + " У вас нет установленных уведомлений"
                }
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                msg.ReplyMarkup = showNotificationsMenu()
                bot.Send(msg)
                UsersMutex.Unlock()
                continue
            case 6:
                if update.Message.Text == "cancel" {
                    text := fmt.Sprintf("Удаление напоминания отменено.")
                    if len(Users[i].Time) > 0 {
                        text = addAllTimeToText(Users[i], text)
                    } else {
                        text = text + " У вас нет установленных уведомлений"
                    }
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                    msg.ReplyMarkup = showNotificationsMenu()
                    bot.Send(msg)
                    Users[i].State = 0
                    UsersMutex.Unlock()
                    continue
                }
                index, err := strconv.Atoi(update.Message.Text)
                if err != nil {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf(`Пожалуйста, введите только номер напоминания.`))
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
                if index < 1 || index > len(Users[i].Time) {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf(`Данного напоминания не существует, проверьте еще раз.`))
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
                times := Users[i].Time[index-1]
                Users[i].Time = append(Users[i].Time[:index-1], Users[i].Time[index:]...)
                Users[i].State = 0
                if err = SaveUsers(); err != nil {
                    fmt.Printf("Ошибка сохранения пользователя: %v", err)
                    UsersMutex.Unlock()
                    continue
                }
                text := fmt.Sprintf(`Уведомление на %d:%d успешно удалено.`, times.Hour, times.Minute)
                if len(Users[i].Time) > 0 {
                    text = addAllTimeToText(Users[i], text)
                } else {
                    text = text + " У вас нет установленных уведомлений"
                }
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                msg.ReplyMarkup = showNotificationsMenu()
                bot.Send(msg)
                UsersMutex.Unlock()
                continue
            }

            switch update.Message.Text {
            case "Профиль":
                text := fmt.Sprintf("Профиль:\nИмя: %s\nВозраст: %d\nЧасовой пояс: %s",
                    Users[i].Name, Users[i].Age, ts.OffsetToEmoji(Users[i].Offset))
                msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
                msg.ReplyMarkup = showMainMenu(Users[i])
                bot.Send(msg)
                UsersMutex.Unlock()
                continue
            case "Напоминания":
                if len(Users[i].Time) == 0 {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("У вас нет установленных уведомлений."))
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
                text := fmt.Sprintf(`Установленные уведомления:`)
                for j, time := range Users[i].Time {
                    text = fmt.Sprintf("%s\n%d - %d:%d", text, j+1, time.Hour, time.Minute)
                }
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                msg.ReplyMarkup = showNotificationsMenu()
                bot.Send(msg)
                UsersMutex.Unlock()
                continue
            case "Отключить уведомления":
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Отправка уведомлений отключена."))
                Users[i].State = -1
                msg.ReplyMarkup = showMainMenu(Users[i])
                bot.Send(msg)
                if err = SaveUsers(); err != nil {
                    fmt.Printf("Ошибка сохранения пользователя: %v", err)
                    UsersMutex.Unlock()
                    continue
                }
                UsersMutex.Unlock()
                continue
            case "Включить уведомления":
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Отправка уведомлений снова включена!"))
                Users[i].State = 0
                msg.ReplyMarkup = showMainMenu(Users[i])
                bot.Send(msg)
                if err = SaveUsers(); err != nil {
                    fmt.Printf("Ошибка сохранения пользователя: %v", err)
                    UsersMutex.Unlock()
                    continue
                }
                UsersMutex.Unlock()
                continue
            case "Добавить":
                if len(Users[i].Time) > 10 {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("У тебя уже есть 10 уведомлений."))
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Для добавления нового уведомления, напиши время нового уведомления в формате hh:mm. Для отмены, напиши cancel"))
                msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
                bot.Send(msg)
                Users[i].State = 5
                UsersMutex.Unlock()
                continue
            case "Удалить":
                if len(Users[i].Time) == 0 {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("У тебя уже нет уведомлений."))
                    bot.Send(msg)
                    UsersMutex.Unlock()
                    continue
                }
                text := fmt.Sprintf(`Для удаления уведомления, напиши номер удаляемого уведомления.`)
                text = text + "\nДля отмены, напиши cancel"
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                msg.ReplyMarkup = showDeleteMenu(Users[i])
                bot.Send(msg)
                Users[i].State = 6
                UsersMutex.Unlock()
                continue
            case "Назад":
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Главное меню:"))
                msg.ReplyMarkup = showMainMenu(Users[i])
                bot.Send(msg)
                UsersMutex.Unlock()
                continue
            default:
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Привет, %s! <ТЕКСТ>", Users[i].Name))
                msg.ReplyMarkup = showMainMenu(Users[i])
                bot.Send(msg)
                UsersMutex.Unlock()
                continue
            }
        }
    }
}