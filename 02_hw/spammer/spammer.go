package main

import (
	"fmt"
	"log"
	"sort"
	"sync"
)

func RunPipeline(cmds ...cmd) {
	var wg sync.WaitGroup
	in := make(chan interface{})

	for _, c := range cmds {
		out := make(chan interface{})
		wg.Add(1)
		go func(c cmd, in, out chan interface{}) {
			defer wg.Done()
			defer close(out)
			c(in, out)
		}(c, in, out)
		in = out
	}

	wg.Wait()
}

func SelectUsers(in, out chan interface{}) {
	// 	in - string
	// 	out - User
	var wg sync.WaitGroup
	var checkedEmails = sync.Map{}

	for email := range in {
		wg.Add(1)
		go func(email string) {
			defer wg.Done()
			user := GetUser(email)
			if _, loaded := checkedEmails.LoadOrStore(user, struct{}{}); !loaded {
				out <- user
			}

		}(email.(string))
	}

	wg.Wait()
}

func SelectMessages(in, out chan interface{}) {
	// 	in - User
	// 	out - MsgID
	var wg sync.WaitGroup
	usersBatch := []User{}

	for user := range in {
		usersBatch = append(usersBatch, user.(User))
		if len(usersBatch) == GetMessagesMaxUsersBatch {
			wg.Add(1)
			go func(batch []User) {
				defer wg.Done()
				msgs, err := GetMessages(batch...)
				if err != nil {
					log.Printf("Ошибка при получении User: %v", err)
				}
				for _, msg := range msgs {
					out <- msg
				}
			}(usersBatch)
			usersBatch = []User{}
		}
	}

	if len(usersBatch) > 0 {
		wg.Add(1)
		go func(batch []User) {
			defer wg.Done()
			msgs, err := GetMessages(batch...)
			if err != nil {
				log.Printf("Ошибка при получении Messages: %v", err)
			}
			for _, msg := range msgs {
				out <- msg
			}
		}(usersBatch)
	}

	wg.Wait()
}

func CheckSpam(in, out chan interface{}) {
	// in - MsgID
	// out - MsgData
	var wg sync.WaitGroup
	sem := make(chan struct{}, HasSpamMaxAsyncRequests)

	for msgID := range in {
		wg.Add(1)
		sem <- struct{}{}
		go func(id MsgID) {
			defer wg.Done()
			defer func() { <-sem }()
			hasSpam, err := HasSpam(id)
			if err != nil {
				log.Printf("Ошибка при проверке: %v", err)
			}
			out <- MsgData{ID: id, HasSpam: hasSpam}
		}(msgID.(MsgID))
	}

	wg.Wait()
}

func CombineResults(in, out chan interface{}) {
	// in - MsgData
	// out - string
	messages := make([]MsgData, 0, len(in))

	for msg := range in {
		messages = append(messages, msg.(MsgData))
	}

	sort.Slice(messages, func(i, j int) bool {
		if messages[i].HasSpam != messages[j].HasSpam {
			return messages[i].HasSpam
		}
		return messages[i].ID < messages[j].ID
	})

	for _, msg := range messages {
		out <- fmt.Sprintf("%t %d", msg.HasSpam, msg.ID)
	}
}
