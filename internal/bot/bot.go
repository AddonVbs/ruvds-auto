package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"modul/internal/config"
	"modul/internal/probe"
	"modul/internal/ruvds"
)

type Bot struct {
	api   *tgbotapi.BotAPI
	cfg   *config.Config
	ruvds *ruvds.Client
}

func New(cfg *config.Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, err
	}
	return &Bot{api: api, cfg: cfg, ruvds: ruvds.New(cfg.RuvdsToken)}, nil
}

func (b *Bot) Run(ctx context.Context) error {
	log.Printf("bot online as @%s, owner=%d", b.api.Self.UserName, b.cfg.OwnerTGID)

	upd := tgbotapi.NewUpdate(0)
	upd.Timeout = 30
	updates := b.api.GetUpdatesChan(upd)

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return ctx.Err()
		case u := <-updates:
			b.handle(ctx, u)
		}
	}
}

func (b *Bot) handle(ctx context.Context, u tgbotapi.Update) {
	switch {
	case u.Message != nil:
		if u.Message.From.ID != b.cfg.OwnerTGID {
			b.reply(u.Message.Chat.ID, "Доступ запрещён.")
			return
		}
		b.handleMessage(ctx, u.Message)
	case u.CallbackQuery != nil:
		if u.CallbackQuery.From.ID != b.cfg.OwnerTGID {
			b.api.Request(tgbotapi.NewCallback(u.CallbackQuery.ID, "Доступ запрещён."))
			return
		}
		b.handleCallback(ctx, u.CallbackQuery)
	}
}

func (b *Bot) handleMessage(ctx context.Context, m *tgbotapi.Message) {
	text := strings.TrimSpace(m.Text)
	switch {
	case text == "/start", text == "/help":
		b.reply(m.Chat.ID,
			"Привет.\n"+
				"/create — создать сервер ("+strconv.Itoa(b.cfg.IPCount)+" IP) и проверить пингом\n"+
				"/list — нет (используй кнопки под сообщением /create)\n"+
				"/whoami — твой Telegram ID")
	case text == "/whoami":
		b.reply(m.Chat.ID, fmt.Sprintf("ID: %d", m.From.ID))
	case text == "/create":
		b.cmdCreate(ctx, m.Chat.ID)
	default:
		b.reply(m.Chat.ID, "Не понял. /help")
	}
}

func (b *Bot) cmdCreate(ctx context.Context, chatID int64) {
	statusMsg, _ := b.api.Send(tgbotapi.NewMessage(chatID, "Отправляю запрос на создание сервера…"))

	name := fmt.Sprintf("%s-%d", b.cfg.ComputerName, time.Now().Unix())
	createReq := ruvds.ServerCreateReq{
		Datacenter:    b.cfg.Datacenter,
		TariffID:      b.cfg.TariffID,
		OSID:          b.cfg.OSID,
		PaymentPeriod: b.cfg.PaymentPeriod,
		CPU:           b.cfg.CPU,
		RAM:           b.cfg.RAM,
		Drive:         b.cfg.Drive,
		DriveTariffID: b.cfg.DriveTariffID,
		IP:            b.cfg.IPCount,
		ComputerName:  name,
		UserComment:   "created via tg-bot",
	}

	resp, err := b.ruvds.CreateServer(ctx, createReq)
	if err != nil {
		b.editText(chatID, statusMsg.MessageID, "Ошибка создания: "+err.Error())
		return
	}

	b.editText(chatID, statusMsg.MessageID,
		fmt.Sprintf("Сервер #%d, ожидаю готовности (стоимость %.2f ₽)…",
			resp.VirtualServerID, resp.CostRub))

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	act, err := b.ruvds.WaitAction(waitCtx, resp.Action.ID, 5*time.Second)
	if err != nil {
		b.editText(chatID, statusMsg.MessageID,
			fmt.Sprintf("Сервер #%d создан, но ожидание прервано: %v", resp.VirtualServerID, err))
		return
	}
	if act.Status != "success" {
		b.editText(chatID, statusMsg.MessageID,
			fmt.Sprintf("Сервер #%d: действие завершилось со статусом %q", resp.VirtualServerID, act.Status))
		return
	}

	nets, err := b.ruvds.GetNetworks(ctx, resp.VirtualServerID)
	if err != nil {
		b.editText(chatID, statusMsg.MessageID,
			fmt.Sprintf("Сервер #%d готов, но получить IP не удалось: %v", resp.VirtualServerID, err))
		return
	}

	ips := make([]string, 0, len(nets.V4))
	for _, n := range nets.V4 {
		ips = append(ips, n.IPAddress)
	}

	results := probe.CheckAll(ips, 3*time.Second)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Сервер #%d готов. Пароль: <code>%s</code>\n\nIP-адреса:\n", resp.VirtualServerID, resp.Password))
	for _, r := range results {
		mark := "❌"
		extra := ""
		if r.Alive {
			mark = "✅"
			extra = fmt.Sprintf(" (порт %d)", r.Port)
		}
		sb.WriteString(fmt.Sprintf("%s <code>%s</code>%s\n", mark, r.IP, extra))
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = deleteKeyboard(resp.VirtualServerID, false)
	b.api.Send(msg)
	b.api.Send(tgbotapi.NewDeleteMessage(chatID, statusMsg.MessageID))
}

func deleteKeyboard(serverID int, confirm bool) tgbotapi.InlineKeyboardMarkup {
	if confirm {
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Да, удалить", fmt.Sprintf("delyes:%d", serverID)),
				tgbotapi.NewInlineKeyboardButtonData("Отмена", fmt.Sprintf("delno:%d", serverID)),
			),
		)
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("🗑 Удалить сервер #%d", serverID),
				fmt.Sprintf("del:%d", serverID),
			),
		),
	)
}

func (b *Bot) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	parts := strings.SplitN(cb.Data, ":", 2)
	if len(parts) != 2 {
		return
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	switch parts[0] {
	case "del":
		edit := tgbotapi.NewEditMessageReplyMarkup(cb.Message.Chat.ID, cb.Message.MessageID, deleteKeyboard(id, true))
		b.api.Send(edit)
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Подтверди удаление."))

	case "delno":
		edit := tgbotapi.NewEditMessageReplyMarkup(cb.Message.Chat.ID, cb.Message.MessageID, deleteKeyboard(id, false))
		b.api.Send(edit)
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Отменено."))

	case "delyes":
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Удаляю…"))
		_, err := b.ruvds.DeleteServer(ctx, id)
		var note string
		if err != nil {
			note = fmt.Sprintf("\n\n❌ Ошибка удаления: %s", err.Error())
		} else {
			note = fmt.Sprintf("\n\n🗑 Сервер #%d помечен на удаление.", id)
		}
		edit := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, cb.Message.Text+note)
		edit.ParseMode = "HTML"
		b.api.Send(edit)
	}
}

func (b *Bot) reply(chatID int64, text string) {
	b.api.Send(tgbotapi.NewMessage(chatID, text))
}

func (b *Bot) editText(chatID int64, msgID int, text string) {
	b.api.Send(tgbotapi.NewEditMessageText(chatID, msgID, text))
}
