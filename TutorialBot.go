package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-contrib/cors"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

var (
	// Menu texts
	firstMenu  = "<b>Menu 1</b>\n\nA beautiful menu with a shiny inline button."
	secondMenu = "<b>Menu 2</b>\n\nA better menu with even more shiny inline buttons."

	// Button texts
	nextButton     = "Next"
	backButton     = "Back"
	tutorialButton = "Tutorial"
	googleButton   = "Search something ?"
	youtubeButton  = "Watch some video mate :D"

	// Store bot screaming status
	screaming = false
	bot       *tgbotapi.BotAPI

	// Keyboard layout for the first menu. One button, one row
	firstMenuMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(nextButton, nextButton),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(googleButton, "https://www.google.com/"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(youtubeButton, "https://www.youtube.com/"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Buy", "/buy"),
		),
	)

	// Keyboard layout for the second menu. Two buttons, one per row
	secondMenuMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(backButton, backButton),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(tutorialButton, "https://core.telegram.org/bots/api"),
		),
	)

	paidUsers = make(map[string]string)
)

func main() {
	var err error
	bot, err = tgbotapi.NewBotAPI("7394545043:AAG9ML1HEkivYWbrvI-mlI7o11bHe5Ak4-Y")
	if err != nil {
		// Abort if something is wrong
		log.Panic(err)
	}

	// Set this to true to log all interactions with telegram servers
	bot.Debug = false

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Create a new cancellable background context. Calling `cancel()` leads to the cancellation of the context
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	// `updates` is a golang channel which receives telegram updates
	updates := bot.GetUpdatesChan(u)
	// Pass cancellable context to goroutine
	go receiveUpdates(ctx, updates)

	// Start Gin router
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"https://causal-miserably-yeti.ngrok-free.app"}, // Thay đổi URL này thành URL frontend của bạn
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept"},
	}))

	// Define route for creating invoices
	r.POST("/api/generate-invoice", generateInvoice)

	log.Println("Server started at :8080")
	go func() {
		if err := r.Run(":8080"); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Tell the user the bot is online
	log.Println("Start listening for updates. Press enter to stop")
	// Wait for a newline symbol, then cancel handling updates
	bufio.NewReader(os.Stdin).ReadBytes('\n')
	cancel()

}

func receiveUpdates(ctx context.Context, updates tgbotapi.UpdatesChannel) {
	// `for {` means the loop is infinite until we manually stop it
	for {
		select {
		// stop looping if ctx is cancelled
		case <-ctx.Done():
			return
		// receive update from channel and then handle it
		case update := <-updates:
			handleUpdate(update)
		}
	}
}

func handleUpdate(update tgbotapi.Update) {
	switch {
	// Handle messages
	case update.Message != nil:
		handleMessage(update.Message)
		break

	// Handle button clicks
	case update.CallbackQuery != nil:
		handleButton(update.CallbackQuery)
		break

	// Handle pre-checkout queries
	case update.PreCheckoutQuery != nil:
		handlePreCheckoutQuery(update.PreCheckoutQuery)
		break

	// Handle successful payments
	case update.Message != nil && update.Message.SuccessfulPayment != nil:
		handleSuccessfulPayment(update.Message.SuccessfulPayment, update.Message.Chat.ID)
		break
	}
}

func handleMessage(message *tgbotapi.Message) {
	user := message.From
	text := message.Text

	if user == nil {
		return
	}

	// Print to console
	log.Printf("%s wrote %s", user.FirstName, text)

	var err error
	if strings.HasPrefix(text, "/") {
		err = handleCommand(message.Chat.ID, text)
	} else if screaming && len(text) > 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, strings.ToUpper(text))
		// To preserve markdown, we attach entities (bold, italic..)
		msg.Entities = message.Entities
		_, err = bot.Send(msg)
	} else {
		// This is equivalent to forwarding, without the sender's name
		copyMsg := tgbotapi.NewCopyMessage(message.Chat.ID, message.Chat.ID, message.MessageID)
		_, err = bot.CopyMessage(copyMsg)
	}

	if err != nil {
		log.Printf("An error occured: %s", err.Error())
	}
}

// When we get a command, we react accordingly
func handleCommand(chatId int64, command string) error {
	parts := strings.SplitN(command, " ", 2) // Tách lệnh và payload
	cmd := parts[0]
	payload := ""
	if len(parts) > 1 {
		payload = parts[1]
	}

	var err error
	switch cmd {
	case "/scream":
		screaming = true
		break

	case "/whisper":
		screaming = false
		break

	case "/menu":
		err = sendMenu(chatId)
		break

	case "/buy":
		err = createInvoiceForChat(chatId, "Something", " Please pay for something")
		break

	case "/status":
		if payload != "" {
			err = nil
		} else {
			msg := tgbotapi.NewMessage(chatId, "Please provide a payload to check the status. (/status {your_payload})")
			_, err = bot.Send(msg)
		}
		break
	case "/list":
		err = createInvoiceForChat(chatId, "You are buying something", " Please pay the something")
		break

	case "/refund":
		err = sendMenu(chatId)
		break
	}
	return err
}

func handleButton(query *tgbotapi.CallbackQuery) {
	var text string

	markup := tgbotapi.NewInlineKeyboardMarkup()
	message := query.Message

	if query.Data == nextButton {
		text = secondMenu
		markup = secondMenuMarkup
	} else if query.Data == backButton {
		text = firstMenu
		markup = firstMenuMarkup
	}

	callbackCfg := tgbotapi.NewCallback(query.ID, "")
	bot.Send(callbackCfg)

	// Replace menu text and keyboard
	msg := tgbotapi.NewEditMessageTextAndMarkup(message.Chat.ID, message.MessageID, text, markup)
	msg.ParseMode = tgbotapi.ModeHTML
	bot.Send(msg)
}

func sendMenu(chatId int64) error {
	msg := tgbotapi.NewMessage(chatId, firstMenu)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = firstMenuMarkup
	_, err := bot.Send(msg)
	return err
}

func generateInvoice(c *gin.Context) {
	var chatId int64 = 5620316173
	title := "Test Product"
	description := "Test description"
	payload := uuid.NewString()
	currency := "XTR"
	prices := []tgbotapi.LabeledPrice{
		{
			Label:  "Test Product",
			Amount: 1,
		},
	}

	// Use bot.Send to send an invoice directly
	invoice := tgbotapi.NewInvoice(
		chatId,
		title,
		description,
		payload,
		"",
		"StartParam",
		currency,
		prices,
	)

	invoice.SuggestedTipAmounts = []int{} // Optional

	_, err := bot.Send(invoice)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send invoice"})
		return
	}

	invoiceLink := fmt.Sprintf("https://t.me/%s?start=%s", bot.Self.UserName, payload)
	c.JSON(http.StatusOK, gin.H{"invoiceLink": invoiceLink})
}

func createInvoiceForChat(chatId int64, title, description string) error {
	var err error
	payload := uuid.NewString()
	prices := []tgbotapi.LabeledPrice{
		{
			Label:  "Price",
			Amount: 1,
		},
	}
	invoice := tgbotapi.NewInvoice(chatId, title, description, payload,
		"", "StartParam", "XTR",
		prices)

	invoice.SuggestedTipAmounts = []int{}

	_, err = bot.Send(invoice)
	msg := tgbotapi.NewMessage(chatId, "Payload of the transaction: "+payload+"Chat ID: "+strconv.FormatInt(chatId, 10))
	bot.Send(msg)
	return err

}

func handlePreCheckoutQuery(preCheckoutQuery *tgbotapi.PreCheckoutQuery) {
	// Here we can validate the information provided by the user during the checkout.
	// For example, check if the user has the required funds, if the order details are correct, etc.
	if preCheckoutQuery == nil {
		return
	}
	answer := tgbotapi.PreCheckoutConfig{
		PreCheckoutQueryID: preCheckoutQuery.ID,
		OK:                 true,
	}

	if _, err := bot.Send(answer); err != nil {
		log.Printf("Error occurred while responding to pre-checkout query: %s", err.Error())
	}
}

func handleSuccessfulPayment(successfulPayment *tgbotapi.SuccessfulPayment, chatId int64) {
	// Get the payload of the transaction
	payload := successfulPayment.InvoicePayload

	// Update user's balance (giả sử bạn đang lưu trữ số dư user trong map `paidUsers`)
	paidUsers[successfulPayment.ProviderPaymentChargeID] = payload

	// Send a thank you message to the user
	msg := tgbotapi.NewMessage(chatId, "Thank you for your payment! Your balance has been updated.")
	bot.Send(msg)
}
