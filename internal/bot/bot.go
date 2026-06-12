package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"modul/internal/config"
	"modul/internal/service"
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

func (b *Bot) handleMessage(ctx context.Context, m *tgbotapi.Message) {
	text := strings.TrimSpace(m.Text)
	switch {
	case text == "/start", text == "/help":
		b.reply(m.Chat.ID,
			"/create — создать VDS (IP="+strconv.Itoa(b.cfg.IPCount)+") и проверить пингом\n"+
				"/list — активные серверы\n"+
				"/info — ID датацентров и ОС из RuVDS\n"+
				"/whoami — твой Telegram ID")
	case text == "/whoami":
		b.reply(m.Chat.ID, fmt.Sprintf("ID: %d", m.From.ID))
	case text == "/create":
		b.cmdCreate(ctx, m.Chat.ID)
	case text == "/list":
		b.cmdList(m.Chat.ID)
	case text == "/info":
		b.cmdInfo(ctx, m.Chat.ID)
	default:
		b.reply(m.Chat.ID, "Не понял. /help")
	}
}

func (b *Bot) cmdCreate(ctx context.Context, chatID int64) {
	statusMsg, _ := b.api.Send(tgbotapi.NewMessage(chatID, "Отправляю запрос на создание сервера…"))

	res, err := b.svc.Create(ctx)
	if err != nil && res == nil {
		b.editText(chatID, statusMsg.MessageID, "Ошибка создания: "+err.Error())
		return
	}

	header := fmt.Sprintf("Сервер #%d в ДЦ %d готов. Пароль: <code>%s</code>",
		res.Server.VirtualServerID, res.Datacenter, res.Server.Password)
	if res.CostRub > 0 {
		header += fmt.Sprintf(" (стоимость %.2f ₽)", res.CostRub)
	}
	if err != nil {
		header += "\n\n⚠️ " + err.Error()
	}

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n\nIP-адреса:\n")
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
	msg.ReplyMarkup = deleteKeyboard(res.Server.VirtualServerID, false)
	b.api.Send(msg)
	b.api.Send(tgbotapi.NewDeleteMessage(chatID, statusMsg.MessageID))
}

func (b *Bot) cmdList(chatID int64) {
	servers, err := b.svc.ListActive()
	if err != nil {
		b.reply(chatID, "Ошибка БД: "+err.Error())
		return
	}
	if len(servers) == 0 {
		b.reply(chatID, "Активных серверов нет.")
		return
	}
	var sb strings.Builder
	for _, s := range servers {
		sb.WriteString(fmt.Sprintf("#%d (ДЦ %d), IP: ", s.VirtualServerID, s.Datacenter))
		ips := make([]string, 0, len(s.IPs))
		for _, ip := range s.IPs {
			ips = append(ips, ip.Address)
		}
		sb.WriteString(strings.Join(ips, ", "))
		sb.WriteString("\n")
	}
	b.reply(chatID, sb.String())
}

func (b *Bot) cmdInfo(ctx context.Context, chatID int64) {
	rep, err := b.svc.Info(ctx)
	if err != nil {
		b.reply(chatID, "Ошибка: "+err.Error())
		return
	}
	var sb strings.Builder
	sb.WriteString("<b>Датацентры:</b>\n")
	for _, d := range rep.Datacenters {
		sb.WriteString(fmt.Sprintf("id=%d  %s\n   vps_tariffs=%v  drive_tariffs=%v\n",
			d.ID, d.Name, d.VPSTariffs, d.DriveTariffs))
	}
	sb.WriteString("\n<b>ОС:</b>\n")
	for _, o := range rep.OS {
		if !o.IsActive {
			continue
		}
		sb.WriteString(fmt.Sprintf("id=%d  [%s]  %s\n", o.ID, o.Type, o.Name))
	}
	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	b.api.Send(msg)
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
		b.api.Send(tgbotapi.NewEditMessageReplyMarkup(
			cb.Message.Chat.ID, cb.Message.MessageID, deleteKeyboard(id, true)))
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Подтверди удаление."))

	case "delno":
		b.api.Send(tgbotapi.NewEditMessageReplyMarkup(
			cb.Message.Chat.ID, cb.Message.MessageID, deleteKeyboard(id, false)))
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Отменено."))

	case "delyes":
		b.api.Request(tgbotapi.NewCallback(cb.ID, "Удаляю…"))
		var note string
		if err := b.svc.Delete(ctx, id); err != nil {
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
