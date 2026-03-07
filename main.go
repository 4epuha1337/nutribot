package main

import (
	"fmt"
	"log"
	rd "nutribot/reminder"
	ts "nutribot/timestamp"
    "database/sql"
    "nutribot/database"
	"nutribot/types"
	"os"
	"strconv"
	"strings"
	"unicode"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "github.com/joho/godotenv"
)

var  (
    db *sql.DB
    OffsetManager *ts.OffsetManager
    ReminderScheduler *rd.Scheduler
)

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


    user, dbID, err := database.GetUserByTelegramId(db, int(userID))
    if err != nil {
        log.Printf("Ошибка получения пользователя: %v", err)
    }
    
    if user == nil {
        bot.Send(tgbotapi.NewMessage(chatID, "Привет, для начала нужно зарегистрироваться. Для этого напиши /start"))
        return
    }

    switch data {
    case "cancel":
        msg = tgbotapi.NewMessage(chatID, "Удаление напоминания отменено.")
        msg.ReplyMarkup = showNotificationsMenu()
        bot.Send(msg)

        err = database.UpdateUserState(db, user.Id, 0)
        if err != nil {
            log.Printf("Ошибка изменения состояния: %v", err)
            return 
        }

        delMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
        bot.Request(delMsg)
        return
    default:
        index, err := strconv.Atoi(data)
        if err != nil {
            return
        }
        reminders, err := database.GetRemindersWithID(db, dbID)
        if err != nil {
            log.Printf("ошибка получения напоминаний: %v", err)
            msg := tgbotapi.NewMessage(chatID, "Ошибка получения списка напоминаний.")
            bot.Send(msg)
            return
        }
        if index < 1 || index > len(reminders) {
            msg = tgbotapi.NewMessage(chatID, fmt.Sprintf(`Данного напоминания не существует, проверьте еще раз.`))
            bot.Send(msg)
            return
        }

        reminderToDelete := reminders[index-1]
        err = database.DeleteReminderById(db, reminderToDelete.ID)
        if err != nil {
            log.Printf("Ошибка удаления напоминания: %v", err)
            msg := tgbotapi.NewMessage(chatID, "Ошибка при удалении напоминания.")
            bot.Send(msg)
            return
        }
        err = database.UpdateUserState(db, user.Id, 0)
        if err != nil {
            log.Printf("Ошибка изменения состояния: %v", err)
        }

        newReminders, err := database.GetRemindersByUserId(db, dbID)
        if err != nil {
            log.Printf("Ошибка получения обновленных напоминаний: %v", err)
        }

        text := fmt.Sprintf(`Уведомление на %d:%d успешно удалено.`, reminderToDelete.Reminder.Hour, reminderToDelete.Reminder.Minute)
        if len(newReminders) > 0 {
            tempUser := types.User {
                Id: user.Id,
                Time: newReminders,
            }
            text = addAllTimeToText(tempUser, text)
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
    db, err := database.InitDB()
    if err != nil {
        fmt.Println("Ошибка: ", err)
        return
    }
    defer db.Close()
    log.Println("База данных инициализирована")

    OffsetManager = ts.NewOffsetManager()

    if err := database.LoadOffsets(db, OffsetManager); err != nil {
        log.Printf("Предупреждение: не удалось загрузить offset: %v", err)
    }

    token := os.Getenv("BOT_TOKEN")
    bot, err := tgbotapi.NewBotAPI(token)
    if err != nil {
        log.Panic(err)
    }

    ReminderScheduler = rd.NewScheduler(
        bot,
        db,
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

            
            var msg tgbotapi.MessageConfig
            tgID := int(update.Message.From.ID)

            if update.Message.IsCommand() {
                switch update.Message.Command() {
                case "start":
                    user, _, err := database.GetUserByTelegramId(db, tgID)
                    if err != nil {
                        log.Printf("Ошибка получения пользователя: %v", err)
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка. Попробуйте позже")
                        bot.Send(msg)
                        continue
                    }
                    if user != nil {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Привет, %s! Чтобы посмотреть свои уведомления, напиши /list. Для добавления нового уведомления напиши /reminder, для удаления - /delete. Для отмены отправки уведомлений напиши /cancel", user.Name))
                        msg.ReplyMarkup = showMainMenu(*user)
                        bot.Send(msg)
                        continue
                    }
                    _, err = database.CreateUser(db, tgID, int(update.Message.Chat.ID), "", 0, 0, 1)
                    if err != nil {
                        log.Printf("Ошибка создания пользователя: %v", err)
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка при регистрации. Попробуйте позже.")
                        bot.Send(msg)
                        continue
                    }
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, `Привет! Давай познакомимся. Меня зовут Морковка, я буду твоим <ВСТАВИТЬ ТЕКСТ>.
Для начала, давай познакомимся. Как тебя зовут?`)
                    bot.Send(msg)
                    continue
                case "reminder":
                    user, dbID, err := database.GetUserByTelegramId(db, tgID)
                    if err != nil {
                        log.Printf("Ошибка получения пользователя: %v", err)
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка.")
                        bot.Send(msg)
                        continue
                    }
                    if user == nil {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Привет, для начала нужно зарегистрироваться. Для этого напиши /start")
                        bot.Send(msg)
                        continue
                    }

                    count, err := database.CountUserReminders(db, dbID)
                    if err != nil {
                        log.Printf("Ошибка подсчета напоминаний: %v", err)
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка.")
                        bot.Send(msg)
                        continue
                    }

                    if count >= 10 {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("У тебя уже есть 10 уведомлений."))
                        bot.Send(msg)
                        continue
                    }
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Для добавления нового уведомления, напиши время нового уведомления в формате hh:mm. Для отмены, напиши cancel"))
                    msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
                    bot.Send(msg)
                    database.UpdateUserState(db, user.Id, 5)
                    continue
                case "delete":
                    user, dbID, err := database.GetUserByTelegramId(db, tgID)
                    if err != nil {
                        log.Printf("Ошибка получения пользователя: %v", err)
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка.")
                        bot.Send(msg)
                        continue
                    }
                    if user == nil {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Привет, для начала нужно зарегистрироваться. Для этого напиши /start"))
                        bot.Send(msg)
                        continue
                    }
                    reminders, err := database.GetRemindersByUserId(db, dbID)
                    if err != nil {
                        log.Printf("Ошибка получения напоминаний: %v", err)
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка.")
                        bot.Send(msg)
                        continue
                    }
                    if len(reminders) == 0 {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("У тебя уже нет уведомлений."))
                        bot.Send(msg)
                        continue
                    }
                    text := fmt.Sprintf(`Для удаления уведомления, напиши номер удаляемого уведомления, либо нажми на кнопку. Установленные уведомления:`)
                    for j, time := range reminders {
                        text = fmt.Sprintf("%s\n%d - %d:%d", text, j+1, time.Hour, time.Minute)
                    }
                    text = text + "\nДля отмены, напиши cancel"
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                    msg.ReplyMarkup = showDeleteMenu(types.User{Id: user.Id, Time: reminders})
                    bot.Send(msg)
                    database.UpdateUserState(db, user.Id, 6)
                    continue
                case "cancel":
                    user, _, err := database.GetUserByTelegramId(db, tgID)
                    if err != nil {
                        log.Printf("Ошибка получения пользователя: %v", err)
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка.")
                        bot.Send(msg)
                        continue
                    }
                    if user == nil {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Привет, для начала нужно зарегистрироваться. Для этого напиши /start")
                        bot.Send(msg)
                        continue
                    }
                    if user.State == -1 {
                        err = database.UpdateUserState(db, user.Id, 0)
                        if err != nil {
                            log.Printf("Ошибка обновления состояния: %v", err)
                        }
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Отправка уведомлений снова включена!"))
                        msg.ReplyMarkup = showMainMenu(*user)
                        bot.Send(msg)
                        continue
                    }
                    err = database.UpdateUserState(db, user.Id, -1)
                    if err != nil {
                        log.Printf("Ошибка обновления состояния: %v", err)
                    }
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Отправка уведомлений отключена."))
                    msg.ReplyMarkup = showMainMenu(*user)
                    bot.Send(msg)
                    continue
                case "list":
                    user, dbID, err := database.GetUserByTelegramId(db, tgID)
                    if err != nil {
                        log.Printf("Ошибка получения пользователя: %v", err)
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка.")
                        bot.Send(msg)
                        continue
                    }
                    if user == nil {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Привет, для начала нужно зарегистрироваться. Для этого напиши /start"))
                        bot.Send(msg)
                        continue
                    }
                    reminders, err := database.GetRemindersByUserId(db, dbID)
                    if err != nil {
                        log.Printf("Ошибка получения напоминаний: %v", err)
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка.")
                        bot.Send(msg)
                        continue
                    }
                    if len(reminders) == 0 {
                        msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("У вас нет установленных уведомлений."))
                        bot.Send(msg)
                        continue
                    }
                    text := fmt.Sprintf(`Установленные уведомления:`)
                    for j, time := range reminders {
                        text = fmt.Sprintf("%s\n%d - %d:%d", text, j+1, time.Hour, time.Minute)
                    }
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                    msg.ReplyMarkup = showNotificationsMenu()
                    bot.Send(msg)
                    continue
                default:
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Извини, не знаю такой команды")
                    bot.Send(msg)
                    continue
                }
            }

            user, dbID, err := database.GetUserByTelegramId(db, tgID)
            if err != nil {
                log.Printf("Ошибка получения пользователя: %v", err)
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка. Попробуйте позже.")
                bot.Send(msg)
                continue
            }
            if user == nil {
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Привет! Для того, чтобы пользоваться мной, необходимо зарегистрироваться. напиши /start")
                bot.Send(msg)
                continue
            }

            switch user.State {
            case 1:
                if hasWhitespace(update.Message.Text) {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Напиши только имя")
                    bot.Send(msg)
                    continue
                }
    
                err = database.UpdateUserProfile(db, user.Id, user.Offset, update.Message.Text, user.Age)
                if err != nil {
                    log.Printf("Ошибка обновления имени: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка сохранения. Попробуйте позже.")
                    bot.Send(msg)
                    continue
                }
    
                err = database.UpdateUserState(db, user.Id, 2)
                if err != nil {
                    log.Printf("Ошибка обновления состояния: %v", err)
                }
    
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Приятно познакомиться, %s! А сколько тебе лет?", update.Message.Text))
                bot.Send(msg)
                continue

            case 2:
                age, err := strconv.Atoi(update.Message.Text)
                if err != nil || age < 1 || age > 150 {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Пожалуйста, введите корректный возраст (число от 1 до 150)")
                    bot.Send(msg)
                    continue
                }

                err = database.UpdateUserProfile(db, user.Id, user.Offset, user.Name, age)
                if err != nil {
                    log.Printf("Ошибка обновления возраста: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка сохранения. Попробуйте позже.")
                    bot.Send(msg)
                    continue
                }
    
                err = database.UpdateUserState(db, user.Id, 3)
                if err != nil {
                    log.Printf("Ошибка обновления состояния: %v", err)
                }
    
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Для корректной работы также нужно узнать ваш часовой пояс. Введите только сдвиг относительно UTC0 (Для Москвы, Санкт-Петербурга - 3)")
                bot.Send(msg)
                continue

            case 3:
                offset, err := strconv.Atoi(update.Message.Text)
                if err != nil {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Это не похоже на сдвиг, попробуй еще раз!")
                    bot.Send(msg)
                    continue
                }
                if offset > 14 || offset < -12 {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Это неверный сдвиг, он должен быть в промежутке от -12 до 14, попробуй еще раз!")
                    bot.Send(msg)
                    continue
                }
    
                err = database.UpdateUserProfile(db, user.Id, offset, user.Name, user.Age)
                if err != nil {
                    log.Printf("Ошибка обновления offset: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка сохранения. Попробуйте позже.")
                    bot.Send(msg)
                    continue
                }

                err = database.UpdateUserState(db, user.Id, 4)
                if err != nil {
                    log.Printf("Ошибка обновления состояния: %v", err)
                }

                OffsetManager.SetUserOffset(int64(user.Id), offset)
    
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Когда тебе напоминать о приеме пищи? Напиши время в формате hh:mm, несколько раз через запятую, но не более 10")
                bot.Send(msg)
                continue

            case 4:
                if update.Message.Text == "cancel" {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Регистрация отменена.")
                    msg.ReplyMarkup = showMainMenu(*user)
                    bot.Send(msg)
                    database.UpdateUserState(db, user.Id, 0)
                    continue
                }
    
                times, err := ParseTimeString(update.Message.Text)
                if err != nil {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf(`Пожалуйста, укажите время в формате hh:mm, hh:mm
Например: 19:20, 15:40`))
                    bot.Send(msg)
                    continue
                }
    
                err = database.AddReminders(db, int(dbID), times)
                if err != nil {
                    log.Printf("Ошибка сохранения напоминаний: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка сохранения. Попробуйте позже.")
                    bot.Send(msg)
                    continue
                }
    
                err = database.UpdateUserState(db, user.Id, 0)
                if err != nil {
                    log.Printf("Ошибка обновления состояния: %v", err)
                }
    
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Отлично! Я все запомнил, и теперь буду напоминать тебе о необходимости приема пищи!")
                msg.ReplyMarkup = showMainMenu(*user)
                bot.Send(msg)
                continue

            case 5:
                if update.Message.Text == "cancel" {
                    text := "Добавление нового напоминания отменено."
        
                    reminders, err := database.GetRemindersByUserId(db, dbID)
                    if err == nil && len(reminders) > 0 {
                    tempUser := types.User{Id: user.Id, Time: reminders}
                        text = addAllTimeToText(tempUser, text)
                    } else {
                        text = text + " У вас нет установленных уведомлений"
                    }
        
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                    msg.ReplyMarkup = showNotificationsMenu()
                    bot.Send(msg)
                    database.UpdateUserState(db, user.Id, 0)
                    continue
                }
    
                count, err := database.CountUserReminders(db, dbID)
                if err != nil {
                    log.Printf("Ошибка подсчета напоминаний: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка проверки лимита.")
                    bot.Send(msg)
                    continue
                }
                if count >= 10 {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "У тебя уже есть 10 уведомлений.")
                    bot.Send(msg)
                    database.UpdateUserState(db, user.Id, 0)
                    continue
                }
    
                times, err := ParseTimeString(update.Message.Text)
                if err != nil {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf(`Пожалуйста, укажите время в формате hh:mm, hh:mm
Например: 19:20, 15:40`))
                    bot.Send(msg)
                    continue
                }

                err = database.AddReminders(db, int(dbID), times)
                if err != nil {
                    log.Printf("Ошибка сохранения напоминаний: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка сохранения.")
                    bot.Send(msg)
                    continue
                }

                database.UpdateUserState(db, user.Id, 0)

                reminders, _ := database.GetRemindersByUserId(db, dbID)
    
                text := fmt.Sprintf("Новое уведомление на %02d:%02d успешно установлено.", times[0].Hour, times[0].Minute)
                if len(reminders) > 0 {
                    tempUser := types.User{Id: user.Id, Time: reminders}
                    text = addAllTimeToText(tempUser, text)
                }
    
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                msg.ReplyMarkup = showNotificationsMenu()
                bot.Send(msg)
                continue

            case 6:
                if update.Message.Text == "cancel" {
                    text := "Удаление напоминания отменено."
        
                    reminders, err := database.GetRemindersByUserId(db, dbID)
                    if err == nil && len(reminders) > 0 {
                        tempUser := types.User{Id: user.Id, Time: reminders}
                        text = addAllTimeToText(tempUser, text)
                    } else {
                        text = text + " У вас нет установленных уведомлений"
                    }
        
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                    msg.ReplyMarkup = showNotificationsMenu()
                    bot.Send(msg)
                    database.UpdateUserState(db, user.Id, 0)
                    continue
                }
    
                index, err := strconv.Atoi(update.Message.Text)
                if err != nil {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Пожалуйста, введите только номер напоминания.")
                    bot.Send(msg)
                    continue
                }

                remindersWithID, err := database.GetRemindersWithID(db, dbID)
                if err != nil {
                    log.Printf("Ошибка получения напоминаний: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка получения списка.")
                    bot.Send(msg)
                    continue
                }
    
                if index < 1 || index > len(remindersWithID) {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Данного напоминания не существует, проверьте еще раз.")
                    bot.Send(msg)
                    continue
                }

                reminderToDelete := remindersWithID[index-1]
                err = database.DeleteReminderById(db, reminderToDelete.ID)
                if err != nil {
                    log.Printf("Ошибка удаления напоминания: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка удаления.")
                    bot.Send(msg)
                    continue
                }

                database.UpdateUserState(db, user.Id, 0)

                reminders, _ := database.GetRemindersByUserId(db, dbID)
    
                text := fmt.Sprintf(" Уведомление на %02d:%02d успешно удалено.", 
                    reminderToDelete.Reminder.Hour, reminderToDelete.Reminder.Minute)
    
                if len(reminders) > 0 {
                    tempUser := types.User{Id: user.Id, Time: reminders}
                    text = addAllTimeToText(tempUser, text)
                } else {
                    text = text + " У вас нет установленных уведомлений"
                }
    
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                msg.ReplyMarkup = showNotificationsMenu()
                bot.Send(msg)
                continue
            }

            switch update.Message.Text {
            case "Профиль":
                freshUser, _, err := database.GetUserByTelegramId(db, tgID)
                if err != nil {
                    log.Printf("Ошибка получения пользователя: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка.")
                    bot.Send(msg)
                    continue
                }

                text := fmt.Sprintf("Профиль:\nИмя: %s\nВозраст: %d\nЧасовой пояс: %s",
                    freshUser.Name, freshUser.Age, ts.OffsetToEmoji(freshUser.Offset))
                msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
                msg.ReplyMarkup = showMainMenu(*freshUser)
                bot.Send(msg)
                continue

            case "Напоминания":
                reminders, err := database.GetRemindersByUserId(db, dbID)
                if err != nil {
                    log.Printf("Ошибка получения напоминаний: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка получения списка.")
                    bot.Send(msg)
                    continue
                }
    
                if len(reminders) == 0 {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "У вас нет установленных уведомлений.")
                    bot.Send(msg)
                    continue
                }

                text := fmt.Sprintf(`Установленные уведомления:`)
                for j, time := range reminders {
                    text = fmt.Sprintf("%s\n%d - %d:%d", text, j+1, time.Hour, time.Minute)
                }

                msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                msg.ReplyMarkup = showNotificationsMenu()
                bot.Send(msg)
                continue
            case "Отключить уведомления":
                err := database.UpdateUserState(db, user.Id, -1)
                if err != nil {
                    log.Printf("Ошибка отключения уведомлений: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка. Попробуйте позже.")
                    bot.Send(msg)
                    continue
                }

                updatedUser, _, err := database.GetUserByTelegramId(db, tgID)
                if err != nil {
                    log.Printf("Ошибка получения обновленного пользователя: %v", err)
                }

                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Отправка уведомлений отключена."))
                msg.ReplyMarkup = showMainMenu(*updatedUser)
                bot.Send(msg)
                continue
            case "Включить уведомления":
                err := database.UpdateUserState(db, user.Id, 0)
                if err != nil {
                    log.Printf("Ошибка включения уведомлений: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка. Попробуйте позже.")
                    bot.Send(msg)
                    continue
                }

                updatedUser, _, err := database.GetUserByTelegramId(db, tgID)
                if err != nil {
                    log.Printf("Ошибка получения обновленного пользователя: %v", err)
                }

                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Отправка уведомлений снова включена!"))
                msg.ReplyMarkup = showMainMenu(*updatedUser)
                bot.Send(msg)
                continue

            case "Добавить":
                count, err := database.CountUserReminders(db, dbID)
                if err != nil {
                    log.Printf("Ошибка подсчета напоминаний: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка проверки лимита.")
                    bot.Send(msg)
                    continue
                }
    
                if count >= 10 {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "❌ У тебя уже есть 10 уведомлений.")
                    bot.Send(msg)
                    continue
                }

                 err = database.UpdateUserState(db, user.Id, 5)
                if err != nil {
                    log.Printf("Ошибка обновления состояния: %v", err)
                }
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Для добавления нового уведомления, напиши время нового уведомления в формате hh:mm. Для отмены, напиши cancel"))
                msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
                bot.Send(msg)
                continue

            case "Удалить":
                reminders, err := database.GetRemindersByUserId(db, dbID)
                if err != nil {
                    log.Printf("Ошибка получения напоминаний: %v", err)
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка получения списка.")
                    bot.Send(msg)
                    continue
                }
    
                if len(reminders) == 0 {
                    msg = tgbotapi.NewMessage(update.Message.Chat.ID, "❌ У тебя уже нет уведомлений.")
                    bot.Send(msg)
                    continue
                }
                err = database.UpdateUserState(db, user.Id, 6)
                if err != nil {
                    log.Printf("Ошибка обновления состояния: %v", err)
                }

                text := fmt.Sprintf(`Для удаления уведомления, напиши номер удаляемого уведомления, или нажми на кнопку ниже.`)
                text = text + "\nДля отмены, напиши cancel"
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)
                msg.ReplyMarkup = showDeleteMenu(types.User{Id: user.Id, Time: reminders})
                bot.Send(msg)
                continue
            case "Назад":
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Главное меню:"))
                msg.ReplyMarkup = showMainMenu(*user)
                bot.Send(msg)
                continue
            default:
                msg = tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Привет, %s! <ТЕКСТ>", user.Name))
                msg.ReplyMarkup = showMainMenu(*user)
                bot.Send(msg)
                continue
            }
        }
    }
}