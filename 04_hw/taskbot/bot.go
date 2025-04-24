package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"
)

const (
	msgHelp = `
		/tasks - посмотреть все задачи
		/new XXX YYY ZZZ -создать новую задачу
		/assign_$ID - сделать пользователя исполлнителем задачи
		/unassign_$ID - удалить задачу у текущего исполнителя
		/resolve_$ID - выполнить задачу, удалить ее из списка
		/my - показать задачи, которые мне поручены
		/owner - показать задачи, которые были созданы мной
	`
	msgGreeting       = "Привет! Я твой менеджер задач!"
	msgNoTasks        = "Нет задач"
	msgNotAssignee    = "Задача не на вас"
	msgAccepted       = "Принято"
	msgNoYourTasks    = "У вас нет задач"
	msgNoCreatedTasks = "Вы не создавали задачи"
	msgUnknownCommand = "Я не знаю такую команду"
	msgLogNoTasks     = "Задачи не существует"
)

var (
	BotToken   string
	WebhookURL string
)

func init() {
	flag.StringVar(&BotToken, "tg.token", "", "token for telegram")
	flag.StringVar(&WebhookURL, "tg.webhook", "", "webhook addr for telegram")
}

type User struct {
	ID       int64
	UserName string
}
type Task struct {
	ID       int64
	Title    string
	Assignee *User
	Owner    *User
}

type TaskManager struct {
	tasks  map[int64]*Task
	lastID int64
}

func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks:  make(map[int64]*Task),
		lastID: 1,
	}
}

func (tm *TaskManager) getAllTasks(userID int64) string {
	var myResponse string
	rowsCount := 0

	tasks := tm.getSortedTasks()
	if len(tasks) > 0 {
		for _, task := range tasks {
			if rowsCount >= 1 {
				myResponse += "\n\n"
			}
			myResponse += formatTaskResponse(*task, userID)
			rowsCount++
		}
	} else {
		myResponse = msgNoTasks
	}

	return myResponse
}

func (tm *TaskManager) getOwnTasks(userID int64) string {
	var myResponse string
	found := false
	rowsCount := 0

	tasks := tm.getSortedTasks()
	for _, task := range tasks {
		if task.Owner.ID == userID {

			if rowsCount >= 1 {
				myResponse += "\n\n"
			}
			myResponse += formatTaskResponse(*task, userID)
			rowsCount++
			found = true
		}
	}
	if !found {
		myResponse = msgNoCreatedTasks
	}

	return myResponse
}

func (tm *TaskManager) getMyTasks(userID int64) string {
	var myResponse string
	found := false

	tasks := tm.getSortedTasks()
	for _, task := range tasks {
		if task.Assignee != nil && task.Assignee.ID == userID {
			myResponse += fmt.Sprintf("%d. %s by @%s\n/unassign_%d /resolve_%d",
				task.ID, task.Title, task.Owner.UserName, task.ID, task.ID)
			found = true
		}

	}
	if !found {
		myResponse = msgNoYourTasks
	}

	return myResponse
}

func (tm *TaskManager) addTasks(userID int64, userName, commandText string) string {
	var myResponse string

	title := strings.TrimSpace(strings.TrimPrefix(commandText, "/new"))

	task := Task{
		ID:    tm.lastID,
		Title: title,
		Owner: &User{
			ID:       userID,
			UserName: userName,
		},
	}
	tm.tasks[tm.lastID] = &task

	myResponse = fmt.Sprintf(`Задача "%s" создана, id=%d`, title, tm.lastID)

	tm.lastID++
	return myResponse
}

func (tm *TaskManager) assignTasks(text string, userID int64, userName string) (string, string, int64) {
	var myResponse, ownerResponse string
	var ownerReceiverID int64

	task, _ := tm.getTaskByID(text)

	if task == nil {
		return "", "", -1
	}

	if task.Assignee != nil {
		ownerReceiverID = task.Assignee.ID
	} else {
		ownerReceiverID = task.Owner.ID
	}

	task.Assignee = &User{
		ID:       userID,
		UserName: userName,
	}

	myResponse = fmt.Sprintf(`Задача "%s" назначена на вас`, task.Title)

	if userID != task.Owner.ID {
		ownerResponse = fmt.Sprintf(`Задача "%s" назначена на @%s`, task.Title, userName)
	}

	return myResponse, ownerResponse, ownerReceiverID
}

func (tm *TaskManager) unassignTasks(text string, userID int64) (string, string, int64) {
	var myResponse, ownerResponse string
	var ownerReceiverID int64

	task, _ := tm.getTaskByID(text)

	if task == nil {
		return "", "", 0
	}

	ownerReceiverID = task.Owner.ID

	if userID != task.Assignee.ID {
		myResponse = msgNotAssignee
	} else {
		task.Assignee = nil
		myResponse = msgAccepted

		ownerResponse = fmt.Sprintf(`Задача "%s" осталась без исполнителя`, task.Title)
	}

	return myResponse, ownerResponse, ownerReceiverID
}

func (tm *TaskManager) resolveTasks(text string, userID int64, userName string) (string, string, int64) {
	var myResponse, ownerResponse string
	var ownerReceiverID int64

	task, id := tm.getTaskByID(text)

	if task == nil {
		return "", "", 0
	}

	ownerReceiverID = task.Owner.ID
	taskTitle := task.Title

	delete(tm.tasks, id)

	myResponse = fmt.Sprintf(`Задача "%s" выполнена`, taskTitle)

	if userID != ownerReceiverID {
		ownerResponse = fmt.Sprintf(`Задача "%s" выполнена @%s`, taskTitle, userName)
	}

	return myResponse, ownerResponse, ownerReceiverID
}

func (tm *TaskManager) getTaskByID(text string) (*Task, int64) {
	parts := strings.Split(text, "_")
	if len(parts) < 2 {
		log.Println("Некорректный формат команды — не найден ID")
		return nil, 0
	}

	assignID, err := strconv.Atoi(parts[1])
	if err != nil {
		log.Printf("Ошибка конвертации string to int: %v", err)
	}

	id := int64(assignID)
	task, taskExists := tm.tasks[id]

	if !taskExists {
		log.Println(msgLogNoTasks)
		return nil, 0
	}

	return task, id
}

func (tm *TaskManager) getSortedTasks() []*Task {
	tasks := make([]*Task, 0, len(tm.tasks))
	for _, task := range tm.tasks {
		tasks = append(tasks, task)
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	return tasks
}

func formatTaskResponse(task Task, userID int64) string {
	if task.Assignee == nil {
		return fmt.Sprintf(
			"%d. %s by @%s\n/assign_%d",
			task.ID,
			task.Title,
			task.Owner.UserName,
			task.ID,
		)
	}
	if task.Assignee.ID == userID {
		return fmt.Sprintf(
			"%d. %s by @%s\nassignee: я\n/unassign_%d /resolve_%d",
			task.ID,
			task.Title,
			task.Owner.UserName,
			task.ID,
			task.ID,
		)
	}

	return fmt.Sprintf(
		"%d. %s by @%s\nassignee: @%s",
		task.ID,
		task.Title,
		task.Owner.UserName,
		task.Assignee.UserName,
	)
}

func startHTTPServer() {
	http.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("all is working")); err != nil {
			log.Printf("Ошибка записи в ResponseWriter: %v", err)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	log.Fatalln("http err:", http.ListenAndServe(":"+port, nil))
	fmt.Println("start listen :" + port)
}

func setupWebhook(bot *tgbotapi.BotAPI) error {
	wh, err := tgbotapi.NewWebhook(WebhookURL)
	if err != nil {
		return fmt.Errorf("NewWebhook failed: %w", err)
	}

	if _, err = bot.Request(wh); err != nil {
		return fmt.Errorf("SetWebhook failed: %w", err)
	}

	return nil
}

func startTaskBot(ctx context.Context) error {
	// сюда пишите ваш код
	bot, err := tgbotapi.NewBotAPI(BotToken)
	if err != nil {
		log.Fatalf("NewBotAPI failed: %s", err)
		return fmt.Errorf("NewBotAPI failed: %w", err)
	}

	bot.Debug = true
	fmt.Printf("Authorized on account %s\n", bot.Self.UserName)

	if err := setupWebhook(bot); err != nil {
		log.Fatalf("Webhook setup failed: %s", err)
	}

	go startHTTPServer()

	// updateConfig := tgbotapi.NewUpdate(0)
	// updateConfig.Timeout = 60

	// updates := bot.GetUpdatesChan(updateConfig)

	updates := bot.ListenForWebhook("/")

	manager := NewTaskManager()

	// Создаём канал для завершения работы
	done := make(chan struct{})

	go func() {
		<-ctx.Done()
		fmt.Println("Получен сигнал завершения, останавливаем бота...")
		close(done)

	}()

	for update := range updates {
		if update.Message != nil {

			log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
			var myResponse, ownerResponse string
			var msg, ownerMsg tgbotapi.MessageConfig
			var receiverID, ownerReceiverID int64

			userID := update.Message.From.ID
			userName := update.Message.From.UserName
			text := update.Message.Command()
			receiverID = update.Message.Chat.ID
			switch {
			case text == "start":
				myResponse = msgGreeting

			case text == "help":
				myResponse = fmt.Sprintf("Вот мои команды: %s", msgHelp)

			case text == "tasks":
				myResponse = manager.getAllTasks(userID)

			case text == "owner":
				myResponse = manager.getOwnTasks(userID)

			case text == "my":
				myResponse = manager.getMyTasks(userID)

			case strings.HasPrefix(text, "new"):
				myResponse = manager.addTasks(userID, userName, update.Message.Text)

			case strings.HasPrefix(text, "assign"):
				myResponse, ownerResponse, ownerReceiverID = manager.assignTasks(text, userID, userName)

			case strings.HasPrefix(text, "unassign"):
				myResponse, ownerResponse, ownerReceiverID = manager.unassignTasks(text, userID)

			case strings.HasPrefix(text, "resolve"):
				myResponse, ownerResponse, ownerReceiverID = manager.resolveTasks(text, userID, userName)

			default:
				myResponse = msgUnknownCommand

			}

			msg = tgbotapi.NewMessage(receiverID, myResponse)
			if _, err := bot.Send(msg); err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}
			if ownerResponse != "" {
				ownerMsg = tgbotapi.NewMessage(ownerReceiverID, ownerResponse)
				if _, err := bot.Send(ownerMsg); err != nil {
					log.Printf("Ошибка отправки сообщения владельцу: %v", err)
				}
			}
		}
	}

	fmt.Println("Бот завершает работу")
	return nil
}

func main() {
	flag.Parse()
	ctx := context.Background()

	err := startTaskBot(ctx)
	if err != nil {
		panic(err)
	}
}
