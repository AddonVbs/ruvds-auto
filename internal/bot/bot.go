package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"modul/internal/config"
	"modul/internal/model"
	"modul/internal/service"
)

const (
	btnCreate = "🆕 Создать"
	btnInfo   = "📋 Инфо"
	btnLogs   = "📜 Логи"
)

type Bot struct {
	api *tgbotapi.BotAPI
	cfg *config.Config
	svc *service.Service
}

func New(cfg *config.Config, svc *service.Service) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, err
	}
	return &Bot{api: api, cfg: cfg, svc: svc}, nil
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

func mainMenu() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnCreate),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnInfo),
			tgbotapi.NewKeyboardButton(btnLogs),
		),
	)
	kb.ResizeKeyboard = true
	return kb
}

func (b *Bot) handleMessage(ctx context.Context, m *tgbotapi.Message) {
	text := strings.TrimSpace(m.Text)
	switch text {
	case "/start", "/help":
		msg := tgbotapi.NewMessage(m.Chat.ID,
			"Привет. Меню снизу.\n"+
				btnCreate+" — новый VDS на "+strconv.Itoa(b.cfg.IPCount)+" IP\n"+
				btnInfo+" — активные серверы\n"+
				btnLogs+" — полная история")
		msg.ReplyMarkup = mainMenu()
		b.api.Send(msg)
	case "/whoami":
		b.reply(m.Chat.ID, fmt.Sprintf("ID: %d", m.From.ID))
	case btnCreate, "/create":
		b.cmdCreate(ctx, m.Chat.ID)
	case btnInfo, "/info":
		b.cmdInfo(ctx, m.Chat.ID)
	case btnLogs, "/logs":
		b.cmdLogs(ctx, m.Chat.ID)
	default:
		b.reply(m.Chat.ID, "Не понял. Жми кнопку в меню.")
	}
}

func (b *Bot) cmdCreate(ctx context.Context, chatID int64) {
	statusMsg, _ := b.api.Send(tgbotapi.NewMessage(chatID, "Отправляю запрос на создание сервера…"))

	res, err := b.svc.Create(ctx)
	if err != nil && res == nil {
		b.editText(chatID, statusMsg.MessageID, "Ошибка создания: "+err.Error())
		return
	}

	dcName := b.svc.DatacenterName(ctx, res.Datacenter)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📍 <b>%s</b>\n", dcName))
	sb.WriteString(fmt.Sprintf("🆔 Сервер #%d", res.Server.VirtualServerID))
	if res.CostRub > 0 {
		sb.WriteString(fmt.Sprintf(" • %.2f ₽", res.CostRub))
	}
	sb.WriteString(fmt.Sprintf("\n🔑 Пароль: <code>%s</code>\n", res.Server.Password))
	if err != nil {
		sb.WriteString("\n⚠️ " + err.Error() + "\n")
	}
	sb.WriteString("\n<b>IP-адреса</b> (тап-удержание = копировать):\n")
	for _, ip := range res.Server.IPs {
		mark := "❌"
		extra := ""
		if ip.Alive {
			mark = "✅"
			extra = fmt.Sprintf(" (порт %d)", ip.Port)
		}
		sb.WriteString(fmt.Sprintf("%s <code>%s</code>%s\n", mark, ip.Address, extra))
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = serverActionsKeyboard(res.Server.VirtualServerID, false)
	b.api.Send(msg)
	b.api.Send(tgbotapi.NewDeleteMessage(chatID, statusMsg.MessageID))
}

func (b *Bot) cmdInfo(ctx context.Context, chatID int64) {
	servers, err := b.svc.ListActive()
	if err != nil {
		b.reply(chatID, "Ошибка БД: "+err.Error())
		return
	}
	if len(servers) == 0 {
		b.reply(chatID, "Активных серверов нет.")
		return
	}
	for _, s := range servers {
		b.api.Send(serverInfoMessage(chatID, &s, b.svc.DatacenterName(ctx, s.Datacenter), false))
	}
}

func (b *Bot) cmdLogs(ctx context.Context, chatID int64) {
	servers, err := b.svc.ListHistory(50)
	if err != nil {
		b.reply(chatID, "Ошибка БД: "+err.Error())
		return
	}
	if len(servers) == 0 {
		b.reply(chatID, "История пуста.")
		return
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>История (%d):</b>\n\n", len(servers)))
	for _, s := range servers {
		state := "🟢"
		if s.DeletedAt != nil {
			state = "🗑"
		}
		ips := make([]string, 0, len(s.IPs))
		for _, ip := range s.IPs {
			ips = append(ips, ip.Address)
		}
		sb.WriteString(fmt.Sprintf("%s <b>#%d</b> • %s\n", state, s.VirtualServerID, b.svc.DatacenterName(ctx, s.Datacenter)))
		sb.WriteString(fmt.Sprintf("   создан: %s\n", s.CreatedAt.Format("2006-01-02 15:04")))
		if s.DeletedAt != nil {
			sb.WriteString(fmt.Sprintf("   удалён: %s\n", s.DeletedAt.Format("2006-01-02 15:04")))
		}
		if len(ips) > 0 {
			sb.WriteString("   IP: <code>" + strings.Join(ips, "</code>, <code>") + "</code>\n")
		}
		sb.WriteString("\n")
	}
	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	b.api.Send(msg)
}

func serverInfoMessage(chatID int64, s *model.Server, dcName string, confirm bool) tgbotapi.MessageConfig {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📍 <b>%s</b>\n", dcName))
	sb.WriteString(fmt.Sprintf("🆔 Сервер #%d\n", s.VirtualServerID))
	sb.WriteString(fmt.Sprintf("🔑 <code>%s</code>\n\n", s.Password))
	sb.WriteString("<b>IP:</b>\n")
	for _, ip := range s.IPs {
		mark := "🟢"
		if !ip.Alive {
			mark = "⚪"
		}
		sb.WriteString(fmt.Sprintf("%s <code>%s</code>\n", mark, ip.Address))
	}
	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = serverActionsKeyboard(s.VirtualServerID, confirm)
	return msg
}

func serverActionsKeyboard(serverID int, confirm bool) tgbotapi.InlineKeyboardMarkup {
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
			tgbotapi.NewInlineKeyboardButtonData("📋 IP списком", fmt.Sprintf("ips:%d", serverID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("🗑 Удалить #%d", serverID),
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
	case "ips":
		srv, err := b.svc.GetByVirtualServerID(id)
		if err != nil {
			b.api.Request(tgbotapi.NewCallback(cb.ID, "Не нашёл сервер: "+err.Error()))
			return
		}
		var sb strings.Builder
		for _, ip := range srv.IPs {
			sb.WriteString("<code>" + ip.Address + "</code>\n")
		}
		msg := tgbotapi.NewMessage(cb.Message.Chat.ID, sb.String())
		msg.ParseMode = "HTML"
		b.api.Send(msg)
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Тап-удержание на IP, чтобы скопировать"))

	case "del":
		b.api.Send(tgbotapi.NewEditMessageReplyMarkup(
			cb.Message.Chat.ID, cb.Message.MessageID, serverActionsKeyboard(id, true)))
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Подтверди удаление."))

	case "delno":
		b.api.Send(tgbotapi.NewEditMessageReplyMarkup(
			cb.Message.Chat.ID, cb.Message.MessageID, serverActionsKeyboard(id, false)))
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Отменено."))

	case "delyes":
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Удаляю…"))
		var note string
		if err := b.svc.Delete(ctx, id); err != nil {
			note = fmt.Sprintf("\n\n❌ Ошибка удаления: %s", err.Error())
		} else {
			note = fmt.Sprintf("\n\n🗑 Сервер #%d помечен на удаление.", id)
		}
		// HTML-сообщение собирается из исходного текста + примечание;
		// reply_markup убираем — кнопки больше не актуальны.
		edit := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, cb.Message.Text+note)
		edit.ParseMode = "HTML"
		empty := tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
		edit.ReplyMarkup = &empty
		b.api.Send(edit)
	}
}

func (b *Bot) reply(chatID int64, text string) {
	b.api.Send(tgbotapi.NewMessage(chatID, text))
}

func (b *Bot) editText(chatID int64, msgID int, text string) {
	b.api.Send(tgbotapi.NewEditMessageText(chatID, msgID, text))
}
