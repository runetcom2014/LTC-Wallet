package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// riteEntry — один обряд в UI
type riteEntry struct {
	id       int
	riteType string
	bits     *widget.Label
	row      *fyne.Container
}

// WalletUI — главное окно приложения
type WalletUI struct {
	app    fyne.App
	window fyne.Window
	ritual *Ritual
	config Config

	// экран ритуала
	riteList    []*riteEntry
	totalBits   *widget.Label
	finalizeBtn *widget.Button
	riteBox     *fyne.Container

	// экран кошелька
	addressLabel  *widget.Label
	mnemonicLabel *widget.Label
	balanceLabel  *widget.Label
	serverLabel   *widget.Label
	refreshBtn    *widget.Button

	// текущий кошелёк
	currentWallet *WalletKeys
	electrum      *ElectrumClient
	currentServer string
}

// RunUI запускает интерфейс
func RunUI(cfg Config) {
	a := app.New()
	a.Settings().SetTheme(newRunicTheme())
	w := a.NewWindow("LTC Wallet")
	w.Resize(fyne.NewSize(700, 780))

	ui := &WalletUI{
		app:    a,
		window: w,
		ritual: NewRitual(),
		config: cfg,
	}
	defer ui.ritual.Free()

	w.SetContent(ui.buildRitualScreen())
	w.ShowAndRun()
}

// ── Экран ритуала ──────────────────────────────────────────────

func (ui *WalletUI) buildRitualScreen() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("Ritual Protocol — LTC Wallet",
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	addStr := widget.NewButtonWithIcon("+ Строка", theme.DocumentIcon(), func() {
		ui.addStringRite()
	})
	addFile := widget.NewButtonWithIcon("+ Файл", theme.FolderOpenIcon(), func() {
		ui.addFileRite()
	})
	addCity := widget.NewButtonWithIcon("+ Город и время", theme.SearchIcon(), func() {
		ui.addCityTimeRite()
	})
	addSeq := widget.NewButtonWithIcon("+ Последовательность", theme.ListIcon(), func() {
		ui.addSequenceRite()
	})
	addRune := widget.NewButtonWithIcon("+ Руны", theme.GridIcon(), func() {
		ui.addRuneGridRite()
	})
	addConst := widget.NewButtonWithIcon("+ Созвездие", theme.ViewFullScreenIcon(), func() {
		ui.addConstellationRite()
	})

	addButtons := container.NewGridWithColumns(3, addStr, addFile, addCity, addSeq, addRune, addConst)
	ui.riteBox = container.NewVBox()

	ui.totalBits = widget.NewLabelWithStyle("Энтропия: 0.0 бит",
		fyne.TextAlignCenter, fyne.TextStyle{})

	ui.finalizeBtn = widget.NewButtonWithIcon(
		"Открыть кошелёк", theme.LoginIcon(), ui.onFinalize)
	ui.finalizeBtn.Importance = widget.HighImportance

	scroll := container.NewVScroll(ui.riteBox)
	scroll.SetMinSize(fyne.NewSize(0, 320))

	return container.NewBorder(
		container.NewVBox(title, addButtons),
		container.NewVBox(ui.totalBits, ui.finalizeBtn),
		nil, nil,
		scroll,
	)
}

// ── STRING обряд ──────────────────────────────────────────────

func (ui *WalletUI) addStringRite() {
	id, err := ui.ritual.AddRite("STRING")
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	entry := &riteEntry{id: id, riteType: "STRING"}

	label := widget.NewLabelWithStyle("Строка", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	entry.bits = widget.NewLabel("0 бит")

	input := widget.NewPasswordEntry()
	input.SetPlaceHolder("Введите секретную фразу...")
	input.OnChanged = func(s string) {
		ui.updateRite(entry, []interface{}{s})
	}

	removeBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		ui.removeRite(entry)
	})

	header := container.NewBorder(nil, nil, label, container.NewHBox(entry.bits, removeBtn))
	entry.row = container.NewVBox(header, input)
	ui.appendRite(entry)
}

// ── FILE обряд ──────────────────────────────────────────────

func (ui *WalletUI) addFileRite() {
	id, err := ui.ritual.AddRite("FILE")
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	entry := &riteEntry{id: id, riteType: "FILE"}

	label := widget.NewLabelWithStyle("Файл", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	entry.bits = widget.NewLabel("0 бит")

	pathEntry := widget.NewEntry()
	pathEntry.SetPlaceHolder("Путь к файлу...")

	offsetEntry := widget.NewEntry()
	offsetEntry.SetText("0")

	saltEntry := widget.NewPasswordEntry()
	saltEntry.SetPlaceHolder("Соль (секретная фраза для файла)...")

	update := func() {
		if pathEntry.Text == "" {
			return
		}
		offset, _ := strconv.ParseInt(offsetEntry.Text, 10, 64)

		// Читаем 512 байт из файла по смещению
		sliceB64, err := readFileSlice(pathEntry.Text, offset, 512)
		if err != nil {
			return
		}

		salt := saltEntry.Text
		filename := pathEntry.Text
		ui.updateRite(entry, []interface{}{sliceB64, salt, filename, float64(offset)})
	}
	pathEntry.OnChanged = func(_ string) { update() }
	offsetEntry.OnChanged = func(_ string) { update() }
	saltEntry.OnChanged = func(_ string) { update() }

	browseBtn := widget.NewButtonWithIcon("Выбрать", theme.FolderOpenIcon(), func() {
		dialog.ShowFileOpen(func(f fyne.URIReadCloser, err error) {
			if err != nil || f == nil {
				return
			}
			pathEntry.SetText(f.URI().Path())
			f.Close()
			update()
		}, ui.window)
	})

	removeBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		ui.removeRite(entry)
	})

	header := container.NewBorder(nil, nil, label, container.NewHBox(entry.bits, removeBtn))
	pathRow := container.NewBorder(nil, nil, nil, browseBtn, pathEntry)
	offsetRow := container.NewGridWithColumns(2, widget.NewLabel("Смещение (байты):"), offsetEntry)
	saltRow := container.NewVBox(widget.NewLabel("Соль:"), saltEntry)

	entry.row = container.NewVBox(header, pathRow, offsetRow, saltRow)
	ui.appendRite(entry)
}

// ── CITYTIME обряд ──────────────────────────────────────────────

func (ui *WalletUI) addCityTimeRite() {
	id, err := ui.ritual.AddRite("CITYTIME")
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	entry := &riteEntry{id: id, riteType: "CITYTIME"}

	allCities := GetCityList()
	var filtered []string
	selectedCity := ""

	label := widget.NewLabelWithStyle("Город и время", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	entry.bits = widget.NewLabel("0 бит")

	removeBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		ui.removeRite(entry)
	})
	header := container.NewBorder(nil, nil, label, container.NewHBox(entry.bits, removeBtn))

	timeEntry := widget.NewEntry()
	timeEntry.SetPlaceHolder("ЧЧ:ММ (напр. 14:30)")

	update := func() {
		if selectedCity == "" || timeEntry.Text == "" {
			return
		}
		digits := ""
		for _, c := range timeEntry.Text {
			if c >= '0' && c <= '9' {
				digits += string(c)
			}
		}
		if len(digits) != 4 {
			return
		}
		hhmm, _ := strconv.ParseFloat(digits, 64)
		ui.updateRite(entry, []interface{}{selectedCity, hhmm})
	}
	timeEntry.OnChanged = func(_ string) { update() }

	selectedLabel := widget.NewLabel("")
	timeRow := container.NewGridWithColumns(2, widget.NewLabel("Время:"), timeEntry)

	// поисковый блок — поле + список
	var cityList *widget.List
	cityList = widget.NewList(
		func() int { return len(filtered) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(filtered[i])
		},
	)

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Начните вводить город...")

	cityScroll := container.NewVScroll(cityList)
	cityScroll.SetMinSize(fyne.NewSize(0, 120))
	searchBlock := container.NewVBox(searchEntry, cityScroll)

	// после выбора города прячем поисковый блок
	cityList.OnSelected = func(i widget.ListItemID) {
		if int(i) >= len(filtered) {
			return
		}
		selectedCity = filtered[i]
		selectedLabel.SetText("✓ " + selectedCity)
		searchBlock.Hide()
		selectedLabel.Show()
		entry.row.Refresh()
		update()
	}

	searchEntry.OnChanged = func(s string) {
		if s == "" {
			filtered = nil
			cityList.Refresh()
			return
		}
		q := strings.ToLower(s)
		filtered = filtered[:0]
		for _, c := range allCities {
			if strings.HasPrefix(strings.ToLower(c), q) {
				filtered = append(filtered, c)
				if len(filtered) >= 50 {
					break
				}
			}
		}
		cityList.Refresh()
	}

	// по клику на выбранный город — возвращаем поиск
	selectedLabel.Hide()
	changeBtn := widget.NewButtonWithIcon("Изменить", theme.SearchIcon(), func() {
		selectedCity = ""
		selectedLabel.Hide()
		searchEntry.SetText("")
		filtered = nil
		cityList.Refresh()
		searchBlock.Show()
		entry.row.Refresh()
	})
	selectedRow := container.NewBorder(nil, nil, nil, changeBtn, selectedLabel)

	entry.row = container.NewVBox(header, searchBlock, selectedRow, timeRow)
	ui.appendRite(entry)
}

// ── Общие методы ──────────────────────────────────────────────

func (ui *WalletUI) appendRite(entry *riteEntry) {
	ui.riteList = append(ui.riteList, entry)
	ui.riteBox.Add(entry.row)
	ui.riteBox.Refresh()
}

func (ui *WalletUI) updateRite(entry *riteEntry, payload []interface{}) {
	if _, err := ui.ritual.UpdateRite(entry.id, payload); err != nil {
		return
	}
	entropy, _ := ui.ritual.GetEntropy()
	for _, re := range entropy.Rites {
		if re.ID == entry.id {
			entry.bits.SetText(fmt.Sprintf("%.1f бит", re.Bits))
		}
	}
	ui.totalBits.SetText(fmt.Sprintf("Энтропия: %.1f бит", entropy.Total))
}

func (ui *WalletUI) removeRite(entry *riteEntry) {
	if err := ui.ritual.RemoveRite(entry.id); err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	for i, e := range ui.riteList {
		if e.id == entry.id {
			ui.riteList = append(ui.riteList[:i], ui.riteList[i+1:]...)
			break
		}
	}
	ui.riteBox.Remove(entry.row)
	ui.riteBox.Refresh()

	entropy, _ := ui.ritual.GetEntropy()
	ui.totalBits.SetText(fmt.Sprintf("Энтропия: %.1f бит", entropy.Total))
}

func (ui *WalletUI) onFinalize() {
	if len(ui.riteList) == 0 {
		dialog.ShowInformation("Ошибка", "Добавьте хотя бы один обряд", ui.window)
		return
	}
	entropy, _ := ui.ritual.GetEntropy()
	if entropy.Total < 80 {
		dialog.ShowConfirm("Низкая энтропия",
			fmt.Sprintf("Суммарная энтропия %.1f бит — меньше рекомендуемых 80 бит.\nПродолжить?", entropy.Total),
			func(ok bool) {
				if ok {
					ui.doFinalize()
				}
			}, ui.window)
		return
	}
	ui.doFinalize()
}

func (ui *WalletUI) doFinalize() {
	ui.finalizeBtn.Disable()
	ui.finalizeBtn.SetText("Открываем кошелёк...")

	go func() {
		masterKey, _, err := ui.ritual.Finalize()
		if err != nil {
			fyne.Do(func() {
				dialog.ShowError(err, ui.window)
				ui.finalizeBtn.Enable()
				ui.finalizeBtn.SetText("Открыть кошелёк")
			})
			return
		}
		wallet, err := DeriveWallet(masterKey)
		if err != nil {
			fyne.Do(func() {
				dialog.ShowError(err, ui.window)
				ui.finalizeBtn.Enable()
				ui.finalizeBtn.SetText("Открыть кошелёк")
			})
			return
		}
		ui.currentWallet = &wallet
		fyne.Do(func() {
			ui.window.SetContent(ui.buildWalletScreen())
		})
		ui.refreshBalance()
	}()
}

// ── Экран кошелька ────────────────────────────────────────────

func (ui *WalletUI) buildWalletScreen() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("LTC Кошелёк",
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	addrTitle := widget.NewLabelWithStyle("Адрес:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ui.addressLabel = widget.NewLabel(ui.currentWallet.Address)
	ui.addressLabel.Wrapping = fyne.TextWrapBreak
	copyAddrBtn := widget.NewButtonWithIcon("Копировать адрес", theme.ContentCopyIcon(), func() {
		ui.window.Clipboard().SetContent(ui.currentWallet.Address)
	})

	mnemoTitle := widget.NewLabelWithStyle("Мнемоника (24 слова):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ui.mnemonicLabel = widget.NewLabel(ui.currentWallet.Mnemonic)
	ui.mnemonicLabel.Wrapping = fyne.TextWrapBreak
	copyMnemoBtn := widget.NewButtonWithIcon("Копировать мнемонику", theme.ContentCopyIcon(), func() {
		ui.window.Clipboard().SetContent(ui.currentWallet.Mnemonic)
	})

	warning := widget.NewLabelWithStyle(
		"⚠ Мнемоника даёт полный доступ к кошельку — храните её в надёжном месте",
		fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	balanceTitle := widget.NewLabelWithStyle("Баланс:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ui.balanceLabel = widget.NewLabel("Загрузка...")
	ui.serverLabel = widget.NewLabel("")
	ui.refreshBtn = widget.NewButtonWithIcon("Обновить", theme.ViewRefreshIcon(), func() {
		ui.refreshBalance()
	})

	backBtn := widget.NewButtonWithIcon("← Новый ритуал", theme.NavigateBackIcon(), func() {
		ui.ritual.Free()
		ui.ritual = NewRitual()
		ui.riteList = nil
		if ui.electrum != nil {
			ui.electrum.Close()
			ui.electrum = nil
		}
		ui.window.SetContent(ui.buildRitualScreen())
	})

	return container.NewBorder(
		container.NewVBox(title, backBtn),
		nil, nil, nil,
		container.NewVScroll(container.NewVBox(
			addrTitle,
			ui.addressLabel,
			copyAddrBtn,
			widget.NewSeparator(),
			mnemoTitle,
			ui.mnemonicLabel,
			copyMnemoBtn,
			warning,
			widget.NewSeparator(),
			widget.NewButtonWithIcon("Отправить", theme.MailSendIcon(), func() {
				ui.showSendDialog()
			}),
			balanceTitle,
			container.NewHBox(ui.balanceLabel, ui.refreshBtn),
			ui.serverLabel,
			ui.buildHistorySection(),
		)),
	)
}

func (ui *WalletUI) refreshBalance() {
	fyne.Do(func() {
		ui.balanceLabel.SetText("Загрузка...")
		ui.refreshBtn.Disable()
	})

	go func() {
		defer fyne.Do(func() { ui.refreshBtn.Enable() })

		if ui.electrum == nil {
			servers := ui.config.Servers[ui.config.Coin]
			client, server, err := ConnectElectrum(servers)
			if err != nil {
				fyne.Do(func() { ui.balanceLabel.SetText("Нет подключения к сети") })
				return
			}
			ui.electrum = client
			ui.currentServer = server
			fyne.Do(func() { ui.serverLabel.SetText("Сервер: " + server) })
		}

		balance, err := ui.electrum.GetBalance(ui.currentWallet.Address)
		if err != nil {
			ui.electrum.Close()
			ui.electrum = nil
			errMsg := err.Error()
			fyne.Do(func() { ui.balanceLabel.SetText("Ошибка: " + errMsg) })
			return
		}

		confirmed := float64(balance.Confirmed) / 1e8
		unconfirmed := float64(balance.Unconfirmed) / 1e8
		text := fmt.Sprintf("%.8f LTC", confirmed)
		if unconfirmed != 0 {
			text += fmt.Sprintf(" (+ %.8f неподтверждённых)", unconfirmed)
		}
		fyne.Do(func() { ui.balanceLabel.SetText(text) })
		ui.loadHistory()
	}()
}

// showSendDialog открывает диалог отправки LTC
func (ui *WalletUI) showSendDialog() {
	if ui.electrum == nil {
		dialog.ShowInformation("Ошибка", "Нет подключения к сети. Обновите баланс сначала.", ui.window)
		return
	}

	toEntry := widget.NewEntry()
	toEntry.SetPlaceHolder("LTC адрес получателя...")

	amountEntry := widget.NewEntry()
	amountEntry.SetPlaceHolder("Сумма в LTC (напр. 0.5)")

	feeLabel := widget.NewLabel("Комиссия: загрузка...")
	totalLabel := widget.NewLabel("")
	statusLabel := widget.NewLabel("")

	// текущий баланс и комиссия — для кнопки MAX
	var currentBalance float64
	var currentFeePerKb float64

	// обновляем итого при изменении суммы
	updateTotal := func() {
		if currentFeePerKb == 0 {
			return
		}
		var amt float64
		fmt.Sscanf(amountEntry.Text, "%f", &amt)
		if amt <= 0 {
			totalLabel.SetText("")
			return
		}
		feeSat := CalcFee(currentFeePerKb, 1, 2)
		feeLTC := float64(feeSat) / 1e8
		totalLabel.SetText(fmt.Sprintf("Итого спишется: %.8f + %.8f = %.8f LTC", amt, feeLTC, amt+feeLTC))
	}
	amountEntry.OnChanged = func(_ string) { updateTotal() }

	// кнопка MAX
	maxBtn := widget.NewButton("MAX", func() {
		if currentFeePerKb == 0 || currentBalance == 0 {
			return
		}
		feeSat := CalcFee(currentFeePerKb, 1, 1) // 1 output — без сдачи
		maxAmt := currentBalance - float64(feeSat)/1e8
		if maxAmt <= 0 {
			dialog.ShowInformation("Ошибка", "Недостаточно средств для покрытия комиссии", ui.window)
			return
		}
		amountEntry.SetText(fmt.Sprintf("%.8f", maxAmt))
		updateTotal()
	})

	// загружаем баланс и комиссию
	go func() {
		// баланс
		balance, err := ui.electrum.GetBalance(ui.currentWallet.Address)
		if err == nil {
			currentBalance = float64(balance.Confirmed) / 1e8
		}
		// комиссия
		fee, err := ui.electrum.EstimateFee(6)
		if err != nil {
			fyne.Do(func() { feeLabel.SetText("Комиссия: ошибка") })
			return
		}
		currentFeePerKb = fee
		feeSat := CalcFee(fee, 1, 2)
		fyne.Do(func() {
			feeLabel.SetText(fmt.Sprintf("Комиссия: ~%.8f LTC", float64(feeSat)/1e8))
			updateTotal()
		})
	}()

	amountRow := container.NewBorder(nil, nil, nil, maxBtn, amountEntry)

	content := container.NewVBox(
		widget.NewLabelWithStyle("Отправить LTC", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Адрес получателя:"),
		toEntry,
		widget.NewLabel("Сумма (LTC):"),
		amountRow,
		feeLabel,
		totalLabel,
		statusLabel,
	)

	dialog.ShowCustomConfirm("Отправка", "Отправить", "Отмена", content, func(ok bool) {
		if !ok {
			return
		}
		ui.doSend(toEntry.Text, amountEntry.Text, statusLabel)
	}, ui.window)
}

func (ui *WalletUI) doSend(toAddr, amountStr string, statusLabel *widget.Label) {
	if toAddr == "" || amountStr == "" {
		dialog.ShowInformation("Ошибка", "Заполните все поля", ui.window)
		return
	}

	// парсим сумму
	var amountLTC float64
	if _, err := fmt.Sscanf(amountStr, "%f", &amountLTC); err != nil || amountLTC <= 0 {
		dialog.ShowInformation("Ошибка", "Неверная сумма", ui.window)
		return
	}
	amountSat := int64(amountLTC * 1e8)

	go func() {
		fyne.Do(func() { statusLabel.SetText("Получаем UTXOs...") })

		utxos, err := ui.electrum.GetUTXOs(ui.currentWallet.Address)
		if err != nil {
			dialog.ShowError(fmt.Errorf("UTXOs: %w", err), ui.window)
			return
		}
		if len(utxos) == 0 {
			dialog.ShowInformation("Ошибка", "Нет доступных средств для отправки", ui.window)
			return
		}

		fyne.Do(func() { statusLabel.SetText("Рассчитываем комиссию...") })

		feePerKb, err := ui.electrum.EstimateFee(6)
		if err != nil {
			feePerKb = 0.001 // fallback
		}

		// предварительный расчёт с 2 выходами (получатель + сдача)
		feeSat := CalcFee(feePerKb, len(utxos), 2)

		selected, change, err := SelectUTXOs(utxos, amountSat, feeSat)
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}

		// пересчитываем fee с реальным количеством inputs
		nOutputs := 2
		if change == 0 {
			nOutputs = 1
		}
		feeSat = CalcFee(feePerKb, len(selected), nOutputs)
		selected, change, err = SelectUTXOs(utxos, amountSat, feeSat)
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}

		// формируем выходы
		outputs := []TxOutput{{Address: toAddr, Value: amountSat}}
		if change > 0 {
			outputs = append(outputs, TxOutput{
				Address: ui.currentWallet.Address,
				Value:   change,
			})
		}

		fyne.Do(func() { statusLabel.SetText("Подписываем транзакцию...") })

		txHex, err := BuildAndSign(selected, outputs, ui.currentWallet.PrivateKey, ui.currentWallet.PublicKey)
		if err != nil {
			dialog.ShowError(fmt.Errorf("подписание: %w", err), ui.window)
			return
		}

		// показываем подтверждение перед отправкой
		confirmMsg := fmt.Sprintf(
			"Отправить %.8f LTC\nПолучатель: %s\nКомиссия: %.8f LTC\nПродолжить?",
			amountLTC, toAddr, float64(feeSat)/1e8,
		)

		dialog.ShowConfirm("Подтверждение", confirmMsg, func(confirmed bool) {
			if !confirmed {
				return
			}
			fyne.Do(func() { statusLabel.SetText("Отправляем...") })
			go func() {
				txid, err := ui.electrum.Broadcast(txHex)
				if err != nil {
					dialog.ShowError(fmt.Errorf("broadcast: %w", err), ui.window)
					return
				}
				dialog.ShowInformation("Успешно!",
					fmt.Sprintf("Транзакция отправлена!\nTXID: %s", txid), ui.window)
				// обновляем баланс
				ui.refreshBalance()
			}()
		}, ui.window)
	}()
}

// ── SEQUENCE обряд ────────────────────────────────────────────

func (ui *WalletUI) addSequenceRite() {
	id, err := ui.ritual.AddRite("SEQUENCE")
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	entry := &riteEntry{id: id, riteType: "SEQUENCE"}
	ds := GetSequenceDataset()

	label := widget.NewLabelWithStyle("Последовательность", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	entry.bits = widget.NewLabel("0 бит")
	removeBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() { ui.removeRite(entry) })
	header := container.NewBorder(nil, nil, label, container.NewHBox(entry.bits, removeBtn))

	var selected []interface{}
	currentLabel := widget.NewLabel("(пусто)")

	updateSeq := func() {
		var display string
		for _, idx := range selected {
			if i, ok := idx.(float64); ok && int(i) < len(ds.Symbols) {
				sym := ds.Symbols[int(i)]
				if e, ok2 := ds.Emoji[sym]; ok2 {
					display += e + " "
				} else {
					display += sym + " "
				}
			}
		}
		if display == "" {
			display = "(пусто)"
		}
		currentLabel.SetText(display)
		ui.updateRite(entry, []interface{}{selected})
	}

	clearBtn := widget.NewButton("Очистить", func() {
		selected = nil
		updateSeq()
	})

	btnGrid := container.NewGridWithColumns(5)
	for i, sym := range ds.Symbols {
		capturedIdx := float64(i)
		emoji := sym
		if e, ok := ds.Emoji[sym]; ok {
			emoji = e
		}
		btn := NewEmojiButton(emoji, func() {
			selected = append(selected, capturedIdx)
			updateSeq()
		})
		btnGrid.Add(btn)
	}

	entry.row = container.NewVBox(
		header,
		widget.NewLabel("Нажмите символы в нужном порядке:"),
		btnGrid,
		container.NewHBox(currentLabel, clearBtn),
	)
	ui.appendRite(entry)
}

// ── RUNEGRID обряд ────────────────────────────────────────────

// runeLabel возвращает короткую ASCII-метку руны для отображения в UI
// (рунические символы не отображаются в стандартном шрифте Fyne)
var runeLabels = []string{
	"Fe", "Ur", "Th", "An", "Ra", "Ka", "Ge", "Wu",
	"Ha", "Na", "Is", "Je", "Ei", "Pe", "Al", "So",
	"Ti", "Be", "Eh", "Ma", "La", "In", "Da", "Od",
}

func runeLabel(idx int, names []string) string {
	if idx < len(runeLabels) {
		return runeLabels[idx]
	}
	if idx < len(names) {
		n := names[idx]
		if len(n) > 4 {
			return n[:4]
		}
		return n
	}
	return fmt.Sprintf("%d", idx)
}

func (ui *WalletUI) addRuneGridRite() {
	id, err := ui.ritual.AddRite("RUNEGRID")
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	entry := &riteEntry{id: id, riteType: "RUNEGRID"}
	ds := GetRuneDataset()

	label := widget.NewLabelWithStyle("Руны", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	entry.bits = widget.NewLabel("0 бит")
	removeBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() { ui.removeRite(entry) })
	header := container.NewBorder(nil, nil, label, container.NewHBox(entry.bits, removeBtn))

	var placements []interface{}
	selectedRune := -1
	selectedRuneLabel := widget.NewLabel("Выберите руну, затем ячейку")

	gridSize := ds.GridSize
	if gridSize == 0 {
		gridSize = 9
	}

	// объявляем заранее — используется в замыканиях grid кнопок
	var runeBtnRefs []*RuneButton

	gridBtns := make([]*RuneButton, gridSize)
	refreshGrid := func() {
		placed := make(map[int]int) // cell -> runeIdx
		for _, p := range placements {
			if pair, ok := p.([]interface{}); ok && len(pair) == 2 {
				cell := int(pair[0].(float64))
				ri := int(pair[1].(float64))
				placed[cell] = ri
			}
		}
		for i, btn := range gridBtns {
			if ri, ok := placed[i]; ok && ri < len(ds.RuneSVGPaths) {
				btn.SVGPath = ds.RuneSVGPaths[ri]
				btn.Name = ds.RuneNames[ri]
			} else {
				btn.SVGPath = ""
				btn.Name = fmt.Sprintf("%d", i+1)
			}
			btn.Refresh()
		}
	}

	gridContainer := container.NewGridWithColumns(3)
	for i := 0; i < gridSize; i++ {
		cell := i
		btn := NewRuneButton("", fmt.Sprintf("%d", cell+1), func() {
			// проверяем есть ли руна в этой ячейке
			cellFilled := false
			for _, p := range placements {
				if pair, ok := p.([]interface{}); ok && len(pair) == 2 {
					if int(pair[0].(float64)) == cell {
						cellFilled = true
						break
					}
				}
			}

			if selectedRune < 0 {
				// руна не выбрана — если ячейка заполнена, очищаем её
				if cellFilled {
					var newPlacements []interface{}
					for _, p := range placements {
						if pair, ok := p.([]interface{}); ok && len(pair) == 2 {
							if int(pair[0].(float64)) != cell {
								newPlacements = append(newPlacements, p)
							}
						}
					}
					placements = newPlacements
					refreshGrid()
					ui.updateRite(entry, placements)
				}
				return
			}

			// руна выбрана — размещаем в ячейке
			var newPlacements []interface{}
			for _, p := range placements {
				if pair, ok := p.([]interface{}); ok && len(pair) == 2 {
					if int(pair[0].(float64)) != cell {
						newPlacements = append(newPlacements, p)
					}
				}
			}
			newPlacements = append(newPlacements, []interface{}{float64(cell), float64(selectedRune)})
			placements = newPlacements
			refreshGrid()
			ui.updateRite(entry, placements)

			// сбрасываем выбор руны
			if selectedRune < len(runeBtnRefs) && runeBtnRefs[selectedRune] != nil {
				runeBtnRefs[selectedRune].Selected = false
				runeBtnRefs[selectedRune].Refresh()
			}
			selectedRune = -1
			selectedRuneLabel.SetText("Выберите руну, затем ячейку")
		})
		gridBtns[i] = btn
		gridContainer.Add(btn)
	}

	// кнопки выбора руны — SVG если доступны, иначе текст
	runeBtns := container.NewGridWithColumns(6)
	for i, r := range ds.Runes {
		capturedIdx := i
		capturedRune := r
		capturedName := ""
		if capturedIdx < len(ds.RuneNames) {
			capturedName = ds.RuneNames[capturedIdx]
		}

		if capturedIdx < len(ds.RuneSVGPaths) {
			// есть SVG path — рисуем руну красиво
			btn := NewRuneButton(ds.RuneSVGPaths[capturedIdx], capturedName, func() {
				// снимаем выделение с предыдущей
				if selectedRune >= 0 && selectedRune < len(runeBtnRefs) {
					runeBtnRefs[selectedRune].Selected = false
					runeBtnRefs[selectedRune].Refresh()
				}
				selectedRune = capturedIdx
				runeBtnRefs[capturedIdx].Selected = true
				runeBtnRefs[capturedIdx].Refresh()
				selectedRuneLabel.SetText("Выбрана: " + capturedName + " — нажмите ячейку")
			})
			runeBtnRefs = append(runeBtnRefs, btn)
			runeBtns.Add(btn)
		} else {
			// fallback — текстовая кнопка
			lbl := runeLabel(capturedIdx, ds.RuneNames)
			btn := widget.NewButton(lbl, func() {
				selectedRune = capturedIdx
				selectedRuneLabel.SetText("Выбрана: " + capturedName + " (" + capturedRune + ") — нажмите ячейку")
			})
			runeBtnRefs = append(runeBtnRefs, nil)
			runeBtns.Add(btn)
		}
	}

	clearBtn := widget.NewButton("Очистить", func() {
		placements = nil
		// сбрасываем выделение SVG кнопки
		if selectedRune >= 0 && selectedRune < len(runeBtnRefs) && runeBtnRefs[selectedRune] != nil {
			runeBtnRefs[selectedRune].Selected = false
			runeBtnRefs[selectedRune].Refresh()
		}
		selectedRune = -1
		selectedRuneLabel.SetText("Выберите руну, затем ячейку")
		refreshGrid()
		ui.updateRite(entry, placements)
	})

	entry.row = container.NewVBox(
		header,
		widget.NewLabel("Выберите руну:"),
		runeBtns,
		selectedRuneLabel,
		widget.NewLabel("Сетка 3×3:"),
		gridContainer,
		clearBtn,
	)
	ui.appendRite(entry)
}

// ── CONSTELLATION обряд ───────────────────────────────────────

func (ui *WalletUI) addConstellationRite() {
	id, err := ui.ritual.AddRite("CONSTELLATION")
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	entry := &riteEntry{id: id, riteType: "CONSTELLATION"}
	ds := GetConstellationDataset()

	steps := ds.Steps
	if steps == 0 {
		steps = 18
	}

	label := widget.NewLabelWithStyle("Созвездие", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	entry.bits = widget.NewLabel("0 бит")
	removeBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() { ui.removeRite(entry) })
	header := container.NewBorder(nil, nil, label, container.NewHBox(entry.bits, removeBtn))

	rotation := 0
	selectedLabel := widget.NewLabel("Звёзды: нет")
	rotLabel := widget.NewLabel(fmt.Sprintf("Поворот: 0/%d", steps))

	var cw *ConstellationWidget
	var clickOrder []int // порядок кликов

	updatePayload := func() {
		var selectedStars []interface{}
		var names []string
		for _, idx := range clickOrder {
			selectedStars = append(selectedStars, float64(idx))
			if idx < len(ds.Stars) {
				names = append(names, ds.Stars[idx].Name)
			}
		}
		ui.updateRite(entry, []interface{}{float64(rotation), selectedStars})
		rotLabel.SetText(fmt.Sprintf("Поворот: %d/%d", rotation, steps))
		if len(names) == 0 {
			selectedLabel.SetText("Звёзды: нет")
		} else {
			selectedLabel.SetText("Выбраны: " + strings.Join(names, ", "))
		}
	}

	cw = NewConstellationWidget(ds.Stars, steps, func(idx int) {
		if cw.Selected[idx] {
			// снимаем — убираем из clickOrder
			cw.Selected[idx] = false
			newOrder := clickOrder[:0]
			for _, i := range clickOrder {
				if i != idx {
					newOrder = append(newOrder, i)
				}
			}
			clickOrder = newOrder
		} else {
			// добавляем — в конец clickOrder
			cw.Selected[idx] = true
			clickOrder = append(clickOrder, idx)
		}
		cw.Refresh()
		updatePayload()
	})

	rotPrev := widget.NewButton("◀", func() {
		rotation = (rotation - 1 + steps) % steps
		cw.Rotation = rotation
		cw.Refresh()
		updatePayload()
	})
	rotNext := widget.NewButton("▶", func() {
		rotation = (rotation + 1) % steps
		cw.Rotation = rotation
		cw.Refresh()
		updatePayload()
	})

	clearBtn := widget.NewButton("Очистить", func() {
		for i := range cw.Selected {
			cw.Selected[i] = false
		}
		clickOrder = nil
		rotation = 0
		cw.Rotation = 0
		cw.Refresh()
		updatePayload()
	})

	entry.row = container.NewVBox(
		header,
		container.NewHBox(rotPrev, rotLabel, rotNext),
		container.NewCenter(cw),
		selectedLabel,
		clearBtn,
	)
	ui.appendRite(entry)
}

// readFileSlice читает size байт из файла по смещению и возвращает base64
func readFileSlice(path string, offset int64, size int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return "", err
		}
	}
	buf := make([]byte, size)
	n, _ := f.Read(buf)
	if n == 0 {
		return "", fmt.Errorf("файл пуст или смещение за пределами")
	}
	return base64.StdEncoding.EncodeToString(buf[:n]), nil
}

// historyBox хранит контейнер истории для обновления
var historyBox *fyne.Container

// showHistory добавляет секцию истории транзакций в экран кошелька
func (ui *WalletUI) buildHistorySection() fyne.CanvasObject {
	historyTitle := widget.NewLabelWithStyle("Последние транзакции:",
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	historyBox = container.NewVBox(widget.NewLabel("Загрузка..."))
	return container.NewVBox(widget.NewSeparator(), historyTitle, historyBox)
}

// loadHistory загружает историю транзакций в фоне
func (ui *WalletUI) loadHistory() {
	if ui.electrum == nil || historyBox == nil {
		return
	}
	// захватываем клиент локально — он может стать nil пока горутина работает
	client := ui.electrum
	wallet := ui.currentWallet
	go func() {
		items, err := client.GetHistory(wallet.Address, 10)
		if err != nil {
			fyne.Do(func() {
				historyBox.Objects = []fyne.CanvasObject{widget.NewLabel("Ошибка загрузки истории")}
				historyBox.Refresh()
			})
			return
		}
		if len(items) == 0 {
			fyne.Do(func() {
				historyBox.Objects = []fyne.CanvasObject{widget.NewLabel("Транзакций пока нет")}
				historyBox.Refresh()
			})
			return
		}

		rows := make([]fyne.CanvasObject, 0, len(items))
		for _, item := range items {
			txid := item.TxHash
			short := txid
			if len(txid) > 24 {
				short = txid[:16] + "..." + txid[len(txid)-8:]
			}

			// Определяем направление и сумму
			amountStr, incoming := client.ClassifyTx(txid, wallet.Address)
			if incoming {
				amountStr = "+" + amountStr
			} else {
				amountStr = "-" + amountStr
			}

			status := fmt.Sprintf("блок %d", item.Height)
			if item.Height == 0 {
				status = "неподтв."
			}

			label := widget.NewLabel(fmt.Sprintf("%-18s  %-26s  %s", amountStr, short, status))
			label.TextStyle = fyne.TextStyle{Monospace: true}
			txidCopy := item.TxHash
			copyBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
				ui.window.Clipboard().SetContent(txidCopy)
			})
			row := container.NewBorder(nil, nil, nil, copyBtn, label)
			rows = append(rows, row)
		}
		fyne.Do(func() {
			historyBox.Objects = rows
			historyBox.Refresh()
		})
	}()
}