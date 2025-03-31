package main

import (
	"fmt"
	"log"
	"sort"
	"sync"
)

func RunPipeline(cmds ...cmd) {
	var wg sync.WaitGroup
	in := make(chan any)

	for _, c := range cmds {
		out := make(chan any)
		wg.Add(1)
		go func(c cmd, in, out chan any) {
			defer wg.Done()
			defer close(out)
			c(in, out)
		}(c, in, out)
		in = out
	}

	wg.Wait()
}

func SelectUsers(in, out chan any) {
	// 	in - string
	// 	out - User
	var wg sync.WaitGroup
	var checkedEmails = sync.Map{}

	for email := range in {
		wg.Add(1)
		go func(e interface{}) {
			defer wg.Done()

			email, ok := e.(string)
			if !ok {
				log.Printf("Ожидался string, получено %T", e)
				return
			}
			user := GetUser(email)
			if _, loaded := checkedEmails.LoadOrStore(user, struct{}{}); !loaded {
				out <- user
			}

		}(email)
	}

	wg.Wait()
}

func SelectMessages(in, out chan any) {
	// 	in - User
	// 	out - MsgID
	var wg sync.WaitGroup
	usersBatch := make([]User, 0, len(in))

	batchFunc := func(batch ...User) {
		defer wg.Done()
		msgs, err := GetMessages(batch...)
		if err != nil {
			log.Printf("Ошибка при получении: %v", err)
			return
		}
		for _, msg := range msgs {
			out <- msg
		}
	}

	for u := range in {
		user, ok := u.(User)
		if !ok {
			log.Printf("Ожидался User, получено %T", u)
			continue
		}

		usersBatch = append(usersBatch, user)
		if len(usersBatch) == GetMessagesMaxUsersBatch {
			wg.Add(1)
			go batchFunc(usersBatch...)
			usersBatch = make([]User, 0, GetMessagesMaxUsersBatch)
		}
	}

	if len(usersBatch) > 0 {
		// wg.Add(1)
		batchFunc(usersBatch...)
	}

	wg.Wait()
}

func CheckSpam(in, out chan any) {
	// in - MsgID
	// out - MsgData
	var wg sync.WaitGroup
	sem := make(chan struct{}, HasSpamMaxAsyncRequests)

	for mID := range in {
		msgID, ok := mID.(MsgID)
		if !ok {
			log.Printf("Ожидался MsgID, получено %T", mID)
			return
		}
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
		}(msgID)
	}

	wg.Wait()
}

func CombineResults(in, out chan any) {
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
