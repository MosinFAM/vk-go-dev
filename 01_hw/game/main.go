package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const itemTypeKey = "ключ"

type Room interface {
	DescribeShort() string
	DescribeItems() string
	Move(direction string) (Room, string)
	Unlock(direction string) string
}

type Item interface {
	Name() string
	Use(target string) string
}

type SimpleItem struct {
	name     string
	itemType string
}

func (i *SimpleItem) Name() string {
	return i.name
}

func (i *SimpleItem) Use(target string) string {
	if i.itemType == itemTypeKey {
		return fmt.Sprintf("вы используете %s на %s", i.name, target)
	}
	return fmt.Sprintf("%s не к чему применить", i.name)
}

type BasicRoom struct {
	description  string
	exits        map[string]Room
	lockedExits  map[string]bool
	items        map[string][]Item
	inventoryCap int
	extraInfo    string
}

func (r *BasicRoom) DescribeShort() string {
	return r.description
}

func (r *BasicRoom) DescribeItems() string {
	var itemsDescription []string
	locations := []string{"ты находишься на кухне, на столе", "на столе", "на стуле"}

	for _, location := range locations {
		items, ok := r.items[location]
		if ok && len(items) > 0 {
			itemNames := make([]string, len(items))
			for i, item := range items {
				itemNames[i] = item.Name()
			}
			itemsDescription = append(itemsDescription, fmt.Sprintf("%s: %s", location, strings.Join(itemNames, ", ")))
		}
	}

	itemsPart := "пустая комната"
	if len(itemsDescription) > 0 {
		itemsPart = strings.Join(itemsDescription, ", ")
	}

	if r.extraInfo != "" {
		return fmt.Sprintf("%s, %s", itemsPart, strings.TrimSpace(r.extraInfo))
	}
	return itemsPart
}

func (r *BasicRoom) Move(direction string) (Room, string) {
	if locked, exists := r.lockedExits[direction]; exists && locked {
		return r, "дверь закрыта"
	}
	nextRoom, ok := r.exits[direction]
	if !ok || nextRoom == nil {
		return r, fmt.Sprintf("нет пути в %s", direction)
	}
	return nextRoom, ""
}

func (r *BasicRoom) Unlock(direction string) string {
	if locked, exists := r.lockedExits[direction]; exists && locked {
		r.lockedExits[direction] = false
		return "дверь открыта"
	}
	return fmt.Sprintf("нет закрытой двери в направлении %s", direction)
}

func availableExits(exits map[string]Room) string {
	desiredOrder := []string{"кухня", "комната", "улица", "коридор", "домой"}
	orderedExits := make([]string, 0)

	for _, room := range desiredOrder {
		if _, exists := exits[room]; exists {
			orderedExits = append(orderedExits, room)
		}
	}

	return strings.Join(orderedExits, ", ")
}

var currentRoom Room
var inventory []Item
var wearingBackpack bool
var kitchen *BasicRoom

// Команды

type Action interface {
	Execute(parts []string) string
}

type LookAction struct{}

func (a *LookAction) Execute(parts []string) string {
	return currentRoom.DescribeItems() + ". можно пройти - " + availableExits(currentRoom.(*BasicRoom).exits)
}

type MoveAction struct{}

func (a *MoveAction) Execute(parts []string) string {
	if len(parts) < 2 {
		return "куда идти?"
	}
	direction := parts[1]
	nextRoom, err := currentRoom.Move(direction)
	if err != "" {
		return err
	}
	currentRoom = nextRoom
	return currentRoom.DescribeShort() + ". можно пройти - " + availableExits(currentRoom.(*BasicRoom).exits)
}

type TakeAction struct{}

func (a *TakeAction) Execute(parts []string) string {
	if len(parts) < 2 {
		return "что взять?"
	}
	return takeItem(parts[1])
}

type UseAction struct{}

func (a *UseAction) Execute(parts []string) string {
	if len(parts) < 3 {
		return "что применять и на что?"
	}
	return useItem(parts[1], parts[2])
}

type WearBackpackAction struct{}

func (a *WearBackpackAction) Execute(parts []string) string {
	if len(parts) < 2 {
		return "что надеть?"
	}

	if parts[1] == "рюкзак" {
		room := currentRoom.(*BasicRoom)
		for location, items := range room.items {
			for i, item := range items {
				if item.Name() == "рюкзак" {
					wearingBackpack = true
					room.items[location] = append(room.items[location][:i], room.items[location][i+1:]...)
					kitchen.extraInfo = "надо идти в универ"
					return "вы надели: рюкзак"
				}
			}
		}
		return "нет рюкзака"
	}
	return "нечего надевать"
}

// Мапа команд
var actions = map[string]Action{
	"осмотреться": &LookAction{},
	"идти":        &MoveAction{},
	"взять":       &TakeAction{},
	"надеть":      &WearBackpackAction{},
	"применить":   &UseAction{},
}

// Функция обработки команд
func handleCommand(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "неизвестная команда"
	}

	actionName := parts[0]
	action, exists := actions[actionName]
	if !exists {
		return "неизвестная команда"
	}

	return action.Execute(parts)
}

// Функции работы с предметами и комнатами
func takeItem(itemName string) string {
	room := currentRoom.(*BasicRoom)
	if !wearingBackpack {
		return "некуда класть"
	}

	for location, items := range room.items {
		for i, item := range items {
			if item.Name() == itemName {
				inventory = append(inventory, item)
				room.items[location] = append(room.items[location][:i], room.items[location][i+1:]...)
				return fmt.Sprintf("предмет добавлен в инвентарь: %s", itemName)
			}
		}
	}
	return "нет такого"
}

func useItem(itemName string, target string) string {
	for _, item := range inventory {
		if item.Name() == itemName {

			if item.(*SimpleItem).itemType == "ключ" && target == "дверь" {
				return currentRoom.Unlock("улица")
			}

			if item.(*SimpleItem).itemType == "ключ" && target != "дверь" {
				return "не к чему применить"
			}

			return item.Use(target)
		}
	}
	return fmt.Sprintf("нет предмета в инвентаре - %s", itemName)
}

// Инициализация игры
func initGame() {
	kitchen = &BasicRoom{
		description: "кухня, ничего интересного",
		items: map[string][]Item{
			"ты находишься на кухне, на столе": {&SimpleItem{name: "чай"}},
		},
		exits:       make(map[string]Room),
		lockedExits: make(map[string]bool),
		extraInfo:   "надо собрать рюкзак и идти в универ",
	}
	corridor := &BasicRoom{
		description: "ничего интересного",
		items:       map[string][]Item{},
		exits:       make(map[string]Room),
		lockedExits: make(map[string]bool),
	}
	room := &BasicRoom{
		description: "ты в своей комнате",
		items: map[string][]Item{
			"на столе": {&SimpleItem{name: "ключи", itemType: "ключ"}, &SimpleItem{name: "конспекты"}},
			"на стуле": {&SimpleItem{name: "рюкзак"}},
		},
		exits:        make(map[string]Room),
		lockedExits:  make(map[string]bool),
		inventoryCap: 1,
	}
	street := &BasicRoom{
		description: "на улице весна",
		items:       map[string][]Item{},
		exits:       make(map[string]Room),
		lockedExits: make(map[string]bool),
	}

	kitchen.exits["коридор"] = corridor
	corridor.exits["кухня"] = kitchen
	corridor.exits["комната"] = room
	room.exits["коридор"] = corridor
	corridor.exits["улица"] = street
	street.exits["домой"] = corridor
	corridor.lockedExits["улица"] = true

	currentRoom = kitchen
	inventory = []Item{}
	wearingBackpack = false
}

func main() {
	initGame()
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		command := scanner.Text()
		fmt.Println(handleCommand(command))
	}
}
