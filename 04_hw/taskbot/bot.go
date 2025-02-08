package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"
)

const help = `
/tasks - посмотреть все задачи
/new XXX YYY ZZZ -создать новую задачу
/assign_$ID - сделать пользователя исполлнителем задачи
/unassign_$ID - удалить задачу у текущего исполнителя
/resolve_$ID - выполнить задачу, удалить ее из списка
/my - показать задачи, которые мне поручены
/owner - показать задачи, которые были созданы мной
`

var (
	BotToken = flag.String("tg.token", "", "token for telegram")

	WebhookURL = flag.String("tg.webhook", "", "webhook addr for telegram")
	// mu            sync.Mutex

)

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

var taskID int64 = 1
var tasks = make(map[int64]*Task)
var tasksKeys = make([]int64, 0, 5)

// Функция для text == /tasks
func allTasks(tasks map[int64]*Task, tasksKeys []int64, userID int64) string {
	var myResponse string
	rowsCount := 0 // количество строк, для пробелов между ними
	if len(tasks) > 0 {
		for _, taskID := range tasksKeys {
			task, taskExists := tasks[taskID]

			if !taskExists {
				log.Println("Задачи не существует")
				break
			}
			if rowsCount >= 1 {
				myResponse += "\n\n"
			}
			myResponse += formatTaskResponse(*task, userID)
			rowsCount++
		}
	} else {
		myResponse = "Нет задач"
	}

	return myResponse
}

// Функция для text == /owner
func ownTasks(tasks map[int64]*Task, tasksKeys []int64, userID int64) string {
	var myResponse string
	found := false
	rowsCount := 0 // количество строк, для пробелов между ними
	for _, taskID := range tasksKeys {
		task, taskExists := tasks[taskID]

		if !taskExists {
			log.Println("Задачи не существует")
			break
		}
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
		myResponse = "Вы не создавали задачи"
	}

	return myResponse
}

// Функция для text == /my
func myTasks(tasks map[int64]*Task, tasksKeys []int64, userID int64) string {
	var myResponse string
	found := false // Есть ли мои таски
	for _, taskID := range tasksKeys {
		task, taskExists := tasks[taskID]

		if !taskExists {
			log.Println("Задачи не существует")
			break
		}
		if task.Assignee != nil && task.Assignee.ID == userID {
			myResponse += fmt.Sprintf("%d. %s by @%s\n/unassign_%d /resolve_%d",
				taskID, task.Title, task.Owner.UserName, taskID, taskID)
			found = true
		}

	}
	if !found {
		myResponse = "У вас нет задач"
	}

	return myResponse
}

// Функция для text == /new
func newTasks(
	tasks map[int64]*Task, tasksKeys *[]int64, userID int64, userName string, commandText string, taskID *int64,
) string {
	var myResponse string

	title := strings.TrimSpace(strings.TrimPrefix(commandText, "/new"))

	// Создаем таску
	task := Task{
		ID:    *taskID,
		Title: title,
		Owner: &User{
			ID:       userID,
			UserName: userName,
		},
	}
	tasks[*taskID] = &task
	*tasksKeys = append(*tasksKeys, *taskID)

	myResponse = fmt.Sprintf(`Задача "%s" создана, id=%d`, title, *taskID)

	(*taskID)++
	return myResponse
}

// Функция для text == /assign
func assignTasks(
	tasks map[int64]*Task, text string, userID int64, userName string,
) (string, string, int64) {
	var myResponse, ownerResponse string
	var ownerReceiverID int64

	task, _ := taskByID(text, tasks)

	if task == nil {
		return "", "", -1
	}

	// Если был исполнитель задачи
	if task.Assignee != nil {
		ownerReceiverID = task.Assignee.ID
	} else {
		ownerReceiverID = task.Owner.ID
	}

	task.Assignee = &User{
		ID:       userID,
		UserName: userName,
	}

	// Сообщение юзеру
	myResponse = fmt.Sprintf(`Задача "%s" назначена на вас`, task.Title)

	// Второе сообщение
	if userID != task.Owner.ID {
		ownerResponse = fmt.Sprintf(`Задача "%s" назначена на @%s`, task.Title, userName)
	}

	return myResponse, ownerResponse, ownerReceiverID
}

// Функция для text == /unassign
func unassignTasks(tasks map[int64]*Task, text string, userID int64) (string, string, int64) {
	var myResponse, ownerResponse string
	var ownerReceiverID int64

	task, _ := taskByID(text, tasks)

	if task == nil {
		return "", "", 0
	}

	ownerReceiverID = task.Owner.ID

	if userID != task.Assignee.ID {
		myResponse = `Задача не на вас`
	} else {
		// Снимаем исполнителя с задачи
		task.Assignee = nil
		myResponse = `Принято`

		// Сообщение владельцу задачи
		ownerResponse = fmt.Sprintf(`Задача "%s" осталась без исполнителя`, task.Title)
	}

	return myResponse, ownerResponse, ownerReceiverID
}

// Функция для text == /resolve
func resolveTasks(
	tasks map[int64]*Task, text string, userID int64, tasksKeys *[]int64, userName string,
) (string, string, int64) {
	var myResponse, ownerResponse string
	var ownerReceiverID int64

	task, id := taskByID(text, tasks)

	if task == nil {
		return "", "", 0
	}

	ownerReceiverID = task.Owner.ID
	taskTitle := task.Title

	// Удаляем задачу
	delete(tasks, id)
	*tasksKeys = remove(*tasksKeys, id)

	// Сообщение исполнителю задачи
	myResponse = fmt.Sprintf(`Задача "%s" выполнена`, taskTitle)

	// Сообщение владельцу задачи
	if userID != ownerReceiverID {
		ownerResponse = fmt.Sprintf(`Задача "%s" выполнена @%s`, taskTitle, userName)
	}

	return myResponse, ownerResponse, ownerReceiverID
}

func taskByID(text string, tasks map[int64]*Task) (*Task, int64) {
	// Парсим команду, получаем ID
	parts := strings.Split(text, "_")
	assignID, err := strconv.Atoi(parts[1])
	if err != nil {
		log.Printf("Ошибка конвертации string to int: %v", err)
	}

	// Получаем задачу по ID
	id := int64(assignID)
	task, taskExists := tasks[id]

	if !taskExists {
		log.Println("Задачи не существует")
		return nil, 0
	}

	return task, id
}

// Форматирует вывод тасок
func formatTaskResponse(task Task, userID int64) string {
	// Если у задачи нет исполнителя
	if task.Assignee == nil {
		return fmt.Sprintf(
			"%d. %s by @%s\n/assign_%d",
			task.ID,
			task.Title,
			task.Owner.UserName,
			task.ID,
		)
	}
	// Если исполнитель задачи - юзер
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

// Удаляет из слайса элемент по значению
func remove(slice []int64, value int64) []int64 {
	// Создаем новый срез для хранения элементов без удаляемого значения
	newSlice := []int64{}
	for _, v := range slice {
		if v != value {
			newSlice = append(newSlice, v) // Добавляем только те элементы, которые не равны value
		}
	}
	return newSlice
}

// startHTTPServer запускает сервер для обработки вебхуков
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

	go func() {
		log.Fatalln("http err:", http.ListenAndServe(":"+port, nil))
	}()
	fmt.Println("start listen :" + port)
}

// setupWebhook устанавливает вебхук для бота
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
	flag.Parse()
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

	// Создаём канал для завершения работы
	done := make(chan struct{})

	go func() {
		<-ctx.Done()
		fmt.Println("Получен сигнал завершения, останавливаем бота...")
		close(done)

	}()

	for {
		select {
		case update := <-updates:
			if update.Message != nil {

				log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
				var myResponse, ownerResponse string
				var msg, ownerMsg tgbotapi.MessageConfig
				var receiverID, ownerReceiverID int64

				userID := update.Message.From.ID
				userName := update.Message.From.UserName
				text := update.Message.Command()
				switch {
				case text == "start":
					myResponse = `Привет! Я твой менеджер задач!`
					receiverID = update.Message.Chat.ID

				case text == "help":
					myResponse = fmt.Sprintf("Вот мои команды: %s", help)
					receiverID = update.Message.Chat.ID

				case text == "tasks":
					myResponse = allTasks(tasks, tasksKeys, userID)
					receiverID = update.Message.Chat.ID

				case text == "owner":
					myResponse = ownTasks(tasks, tasksKeys, userID)
					receiverID = update.Message.Chat.ID

				case text == "my":
					myResponse = myTasks(tasks, tasksKeys, userID)
					receiverID = update.Message.Chat.ID

				case strings.HasPrefix(text, "new"):
					myResponse = newTasks(tasks, &tasksKeys, userID, userName, update.Message.Text, &taskID)
					receiverID = update.Message.Chat.ID

				case strings.HasPrefix(text, "assign"):
					receiverID = update.Message.Chat.ID
					myResponse, ownerResponse, ownerReceiverID = assignTasks(tasks, text, userID, userName)

				case strings.HasPrefix(text, "unassign"):
					receiverID = update.Message.Chat.ID
					myResponse, ownerResponse, ownerReceiverID = unassignTasks(tasks, text, userID)

				case strings.HasPrefix(text, "resolve"):
					receiverID = update.Message.Chat.ID
					myResponse, ownerResponse, ownerReceiverID = resolveTasks(tasks, text, userID, &tasksKeys, userName)

				default:
					myResponse = `Я не знаю такую команду`
					receiverID = update.Message.Chat.ID

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
		case <-done:
			fmt.Println("Бот завершает работу")
			return nil
		}
	}
}

func main() {
	err := startTaskBot(context.Background())
	if err != nil {
		panic(err)
	}
}
