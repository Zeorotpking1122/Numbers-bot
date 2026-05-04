package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biter777/countries"
	tele "gopkg.in/telebot.v3"
)

// ---------------- CONFIGURATION ---------------- //
const (
	BotToken = "8663830409:AAF51QzoF8xYh6pyjosevnHiBVxbGhhXHgU"
	LogoPath = "logo.png"
)

var AuthorizedUsers = []int64{8382316368, }

// File Paths
const (
	ChannelsFile = "channels.json"
	ServersFile  = "servers.json"
	UsersFile    = "users.json"
	StatsFile    = "stats.json"
)

// ---------------- DATA STRUCTURES ---------------- //

type Server struct {
	Name       string `json:"name"`
	BaseURL    string `json:"base_url,omitempty"`
	APINumbers string `json:"api_numbers"`
	APISMS     string `json:"api_sms"`
	Active     bool   `json:"active"`
}

type Channel struct {
	Name string `json:"name"`
	Link string `json:"link"`
	ID   string `json:"id"`
}

type BotStats struct {
	TotalOTPs   int     `json:"total_otps"`
	CurrentDate string  `json:"current_date"`
	DailyUsers  []int64 `json:"daily_users"`
}

// Global Variables & Mutexes
var (
	botConfig      []Server
	reqChannels    []Channel
	totalUsers     = make(map[int64]bool)
	botStats       BotStats
	userSelections = make(map[int64]map[string]string)

	// Admin Panel State Machine
	adminStates   = make(map[int64]string)
	adminTempData = make(map[int64]map[string]string)

	dataMutex sync.RWMutex

	httpClient = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}
)

// ---------------- HARDCODED INIT ---------------- //
func initData() {
	loadJSON(UsersFile, &totalUsers)

	botStats = BotStats{CurrentDate: time.Now().Format("2006-01-02"), DailyUsers: []int64{}}
	loadJSON(StatsFile, &botStats)

	hardcoded := []Server{
		{Name: "🔥ZERO NP🔥", BaseURL: "https://ali-api-proo.up.railway.app/api/np"},	
		{Name: "❤️ZERO MSI ❤️", BaseURL: "https://ali-api-proo.up.railway.app/api/msi"},
		{Name: "ZERO MAT", BaseURL: "https://ali-api-proo.up.railway.app/api/mat"},
		{Name: "✅ ZERO TIME ✅", BaseURL: "https://ali-api-proo.up.railway.app/api/ts"},
		{Name: "😁 CHOICE 😁", BaseURL: "https://ali-api-proo.up.railway.app/api/ch"},
		{Name: "💖 ZERO GREEN 💖", BaseURL: "https://ali-api-proo.up.railway.app/api/gen"},
		{Name: "💞 IVASMS 💞", BaseURL: "https://ali-api-proo.up.railway.app/api/ivs"},		
	}

	botConfig = make([]Server, len(hardcoded))
	for i, s := range hardcoded {
		baseURL := strings.Split(s.BaseURL, "?")[0]
		botConfig[i] = Server{
			Name:       s.Name,
			APINumbers: baseURL + "?type=numbers",
			APISMS:     baseURL + "?type=sms",
			Active:     true,
		}
	}
	saveJSON(ServersFile, botConfig)

	defaultChannels := []Channel{
		{Name: "ᴢᴇʀᴏᴛʀᴀᴄᴇɴᴜᴍs", Link: "https://t.me/ZeroTraceNums", ID: "-1003233736476"},
		{Name: "𝚣𝚎𝚛𝚘𝚝𝚛𝚊𝚌𝚎𝚗𝚞𝚖𝚜 𝚘𝚝𝚙", Link: "https://t.me/ZeroTraceNums1", ID: "-1003414638512"},
		{Name: "WhatsApp Numbers group", Link: "https://chat.whatsapp.com/LwPIdOAbtmnBUhSr0qbNxg?mode=wwt", ID: ""},
		{Name: "WhatsApp 𝚘𝚝𝚙", Link: "https://whatsapp.com/channel/0029VaSudNI4dTnSwd5Q4K1Z", ID: ""},
	}
	if err := loadJSON(ChannelsFile, &reqChannels); err != nil || len(reqChannels) == 0 {
		reqChannels = defaultChannels
		saveJSON(ChannelsFile, reqChannels)
	}
}

// ---------------- HELPER FUNCTIONS ---------------- //

func loadJSON(filename string, v interface{}) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewDecoder(file).Decode(v)
}

func saveJSON(filename string, v interface{}) {
	dataMutex.Lock()
	defer dataMutex.Unlock()
	data, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		os.WriteFile(filename, data, 0644)
	}
}

func fetchAPI(url string) (map[string]interface{}, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}

func trackUserActivity(userID int64) {
	dataMutex.Lock()
	if !totalUsers[userID] {
		totalUsers[userID] = true
		go saveJSON(UsersFile, totalUsers)
	}
	today := time.Now().Format("2006-01-02")
	if botStats.CurrentDate != today {
		botStats.CurrentDate = today
		botStats.DailyUsers = []int64{}
	}
	found := false
	for _, id := range botStats.DailyUsers {
		if id == userID {
			found = true
			break
		}
	}
	if !found {
		botStats.DailyUsers = append(botStats.DailyUsers, userID)
		go saveJSON(StatsFile, botStats)
	}
	dataMutex.Unlock()
}

func checkSubscription(b *tele.Bot, user *tele.User) bool {
	for _, ch := range reqChannels {
		if ch.ID == "" {
			continue
		}
		chatID, _ := strconv.ParseInt(ch.ID, 10, 64)
		chat, err := b.ChatByID(chatID)
		if err != nil {
			continue
		}
		member, err := b.ChatMemberOf(chat, user)
		if err != nil || (member.Role != tele.Member && member.Role != tele.Administrator && member.Role != tele.Creator) {
			return false
		}
	}
	return true
}

func isAdmin(userID int64) bool {
	for _, id := range AuthorizedUsers {
		if id == userID {
			return true
		}
	}
	return false
}

func fastCleanName(raw string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	raw = re.ReplaceAllString(raw, "")
	parts := strings.Split(strings.ReplaceAll(raw, "_", "-"), "-")
	country := strings.Title(strings.TrimSpace(parts[0]))
	
	if len(parts) > 1 {
		p1 := strings.ToLower(parts[1])
		if strings.ToLower(country) == "ivory" && strings.Contains(p1, "cost") {
			return "Côte d'Ivoire"
		}
		if strings.ToLower(country) == "south" || strings.ToLower(country) == "sri" || strings.ToLower(country) == "new" {
			country = fmt.Sprintf("%s %s", country, strings.Title(strings.TrimSpace(parts[1])))
		}
	}
	if c := strings.ToLower(country); c == "uk" || c == "england" {
		country = "United Kingdom"
	} else if c == "usa" {
		country = "United States"
	}
	reNum := regexp.MustCompile(`\d+`)
	return strings.TrimSpace(reNum.ReplaceAllString(country, ""))
}

func getFlagFromName(countryName string) string {
	c := countries.ByName(countryName)
	if c != countries.Unknown {
		return c.Emoji()
	}
	return "🏳️"
}

func sendMenuWithLogo(c tele.Context, text string, markup *tele.ReplyMarkup) error {
	photo := &tele.Photo{File: tele.FromDisk(LogoPath), Caption: text}
	return c.Send(photo, tele.ModeMarkdown, markup)
}

func editMenuWithLogo(c tele.Context, text string, markup *tele.ReplyMarkup) error {
	photo := &tele.Photo{File: tele.FromDisk(LogoPath), Caption: text}
	err := c.Edit(photo, tele.ModeMarkdown, markup)
	if err != nil {
		c.Delete()
		return sendMenuWithLogo(c, text, markup)
	}
	return nil
}

// ---------------- HISTORY MANAGEMENT ---------------- //

func getHistoryFilename(idx int) string {
	return fmt.Sprintf("history_server_%d.json", idx)
}

func loadHistory(idx int) map[string]bool {
	hist := make(map[string]bool)
	var list []string
	if err := loadJSON(getHistoryFilename(idx), &list); err == nil {
		for _, s := range list {
			hist[s] = true
		}
	}
	return hist
}

func saveHistory(idx int, hist map[string]bool) {
	var list []string
	for k := range hist {
		list = append(list, k)
	}
	go saveJSON(getHistoryFilename(idx), list)
}

func initializeServerHistory() {
	log.Println("🔄 [SYSTEM] Initializing Server History...")
	for idx, server := range botConfig {
		if !server.Active {
			continue
		}
		data, err := fetchAPI(server.APISMS)
		if err != nil || data["aaData"] == nil {
			continue
		}
		currentHistory := make(map[string]bool)
		for _, item := range data["aaData"].([]interface{}) {
			row := item.([]interface{})
			firstCol := fmt.Sprintf("%v", row[0])
			pNum, pMsg := "", ""
			if strings.Contains(firstCol, "<input") || strings.Contains(firstCol, "checkbox") {
				pNum, pMsg = fmt.Sprintf("%v", row[3]), fmt.Sprintf("%v", row[5])
			} else {
				pNum, pMsg = fmt.Sprintf("%v", row[2]), fmt.Sprintf("%v", row[4])
			}
			if len(pMsg) > 1 {
				sig := fmt.Sprintf("%s|%s", strings.TrimSpace(pNum), strings.TrimSpace(pMsg))
				currentHistory[sig] = true
			}
		}
		saveHistory(idx, currentHistory)
	}
	log.Println("✅ [SYNC] History initialized.")
}

func deleteServerHistory(idx int) {
	os.Remove(getHistoryFilename(idx))
}

// ---------------- MAIN HANDLERS ---------------- //

func main() {
	initData()
	go initializeServerHistory()

	pref := tele.Settings{
		Token:  BotToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("🚀 High-Performance Go Bot Started with Admin Panel & Flags!")

	// /start Command
	b.Handle("/start", func(c tele.Context) error {
		trackUserActivity(c.Sender().ID)
		menu := &tele.ReplyMarkup{}
		var rows []tele.Row
		for _, ch := range reqChannels {
			rows = append(rows, menu.Row(menu.URL(fmt.Sprintf("⚜️ Join %s", ch.Name), ch.Link)))
		}
		rows = append(rows, menu.Row(menu.Data("⚡ VERIFY & START", "check_join")))
		menu.Inline(rows...)
		return sendMenuWithLogo(c, "👋 *Welcome! Please join our channels to continue.*", menu)
	})

	// Universal Admin State Handler for text/media
	handleAdminInput := func(c tele.Context) error {
		userID := c.Sender().ID
		state := adminStates[userID]

		if state == "" {
			return nil
		}

		if c.Text() != "" && strings.ToLower(c.Text()) == "cancel" {
			adminStates[userID] = ""
			return c.Send("❌ *Action Cancelled.*", tele.ModeMarkdown)
		}

		switch state {
		case "waiting_for_broadcast":
			adminStates[userID] = ""
			sendMenuWithLogo(c, "🚀 *Broadcast Started...*", nil)
			go func(msg *tele.Message) {
				count := 0
				for uID := range totalUsers {
					user := &tele.User{ID: uID}
					_, err := b.Copy(user, msg)
					if err == nil {
						count++
					}
					time.Sleep(100 * time.Millisecond)
				}
				b.Send(c.Sender(), fmt.Sprintf("✅ *Broadcast Finished!*\nSent to `%d` users.", count), tele.ModeMarkdown)
			}(c.Message())

		case "waiting_for_api_url":
			rawURL := strings.Split(strings.TrimSpace(c.Text()), "?")[0]
			if strings.HasPrefix(rawURL, "http") {
				if adminTempData[userID] == nil {
					adminTempData[userID] = make(map[string]string)
				}
				adminTempData[userID]["api_numbers"] = rawURL + "?type=numbers"
				adminTempData[userID]["api_sms"] = rawURL + "?type=sms"
				adminStates[userID] = "waiting_for_api_name"
				return sendMenuWithLogo(c, "📝 *Enter Server Button Name:*", nil)
			}
			return sendMenuWithLogo(c, "❌ *Invalid URL. Must start with http/https.*", nil)

		case "waiting_for_api_name":
			adminStates[userID] = ""
			name := strings.TrimSpace(c.Text())
			newServer := Server{
				Name:       name,
				APINumbers: adminTempData[userID]["api_numbers"],
				APISMS:     adminTempData[userID]["api_sms"],
				Active:     true,
			}
			botConfig = append(botConfig, newServer)
			saveJSON(ServersFile, botConfig)
			delete(adminTempData, userID)
			return sendMenuWithLogo(c, "✅ *API Added Successfully!*", nil)

		case "waiting_for_channel_target":
			target := strings.TrimSpace(c.Text())
			var chat *tele.Chat
			var err error
			
			if strings.HasPrefix(target, "@") {
				chat, err = b.ChatByUsername(target)
			} else {
				chatID, _ := strconv.ParseInt(target, 10, 64)
				chat, err = b.ChatByID(chatID)
			}
			
			// یہ وہ لائن ہے جو گو کو بتائے گی کہ ہم نے err کو استعمال کر لیا ہے
			if err != nil {
				return sendMenuWithLogo(c, "❌ *Invalid Channel ID/Link.*", nil)
			}
			
			adminStates[userID] = "waiting_for_channel_name"
			if adminTempData[userID] == nil {
				adminTempData[userID] = make(map[string]string)
			}
			adminTempData[userID]["id"] = strconv.FormatInt(chat.ID, 10)
			if chat.Username != "" {
				adminTempData[userID]["link"] = "https://t.me/" + chat.Username
			}
			return sendMenuWithLogo(c, "✅ *Success!*\nNow enter the *Button Name* for this channel:", nil)

		case "waiting_for_channel_name":
			adminTempData[userID]["name"] = strings.TrimSpace(c.Text())
			if adminTempData[userID]["link"] == "" {
				adminStates[userID] = "waiting_for_channel_link"
				return sendMenuWithLogo(c, "🔗 This is a private channel. Please provide a *Join Link*:", nil)
			}
			adminStates[userID] = ""
			reqChannels = append(reqChannels, Channel{
				Name: adminTempData[userID]["name"],
				Link: adminTempData[userID]["link"],
				ID:   adminTempData[userID]["id"],
			})
			saveJSON(ChannelsFile, reqChannels)
			delete(adminTempData, userID)
			return sendMenuWithLogo(c, "✅ *Channel Added Successfully!*", nil)

		case "waiting_for_channel_link":
			adminStates[userID] = ""
			adminTempData[userID]["link"] = strings.TrimSpace(c.Text())
			reqChannels = append(reqChannels, Channel{
				Name: adminTempData[userID]["name"],
				Link: adminTempData[userID]["link"],
				ID:   adminTempData[userID]["id"],
			})
			saveJSON(ChannelsFile, reqChannels)
			delete(adminTempData, userID)
			return sendMenuWithLogo(c, "✅ *Channel Added Successfully!*", nil)
		}
		return nil
	}

	b.Handle(tele.OnText, handleAdminInput)
	b.Handle(tele.OnPhoto, handleAdminInput)
	b.Handle(tele.OnVideo, handleAdminInput)
	b.Handle(tele.OnDocument, handleAdminInput)

	// Callback Router
	b.Handle(tele.OnCallback, func(c tele.Context) error {
		trackUserActivity(c.Sender().ID)
		data := strings.TrimSpace(c.Callback().Data)
		userID := c.Sender().ID

		// Admin Callbacks
		if strings.HasPrefix(data, "admin_") || data == "open_owner_panel" {
			if !isAdmin(userID) {
				return c.Respond(&tele.CallbackResponse{Text: "❌ You are not an admin.", ShowAlert: true})
			}
			return handleAdminCallback(c, b, data)
		}

		// User Callbacks
		switch {
		case data == "check_join":
			if checkSubscription(b, c.Sender()) {
				return showMainMenu(c)
			}
			return c.Respond(&tele.CallbackResponse{Text: "❌ Join all channels first!", ShowAlert: true})
			
		case data == "main_menu":
			return showMainMenu(c)

		case data == "country_random_all":
			var activeServers []int
			for i, s := range botConfig {
				if s.Active {
					activeServers = append(activeServers, i)
				}
			}
			if len(activeServers) == 0 {
				return c.Respond(&tele.CallbackResponse{Text: "❌ No active servers.", ShowAlert: true})
			}
			sIdx := activeServers[rand.Intn(len(activeServers))]
			dataMutex.Lock()
			if userSelections[userID] == nil {
				userSelections[userID] = make(map[string]string)
			}
			userSelections[userID]["server_idx"] = strconv.Itoa(sIdx)
			dataMutex.Unlock()
			return processNumberFetch(c, "random")

		case strings.HasPrefix(data, "set_server_"):
			parts := strings.Split(data, "_")
			idx, _ := strconv.Atoi(parts[len(parts)-1])
			dataMutex.Lock()
			if userSelections[userID] == nil {
				userSelections[userID] = make(map[string]string)
			}
			userSelections[userID]["server_idx"] = strconv.Itoa(idx)
			dataMutex.Unlock()
			return getCountriesMenu(c, idx)

		case strings.HasPrefix(data, "country_") || strings.HasPrefix(data, "change_number_"):
			parts := strings.Split(data, "_")
			country := parts[len(parts)-1]
			return processNumberFetch(c, country)

		case data == "check_otp":
			return checkOTP(c)
		}
		return c.Respond()
	})

	b.Start()
}

// ---------------- ADMIN PANEL LOGIC ---------------- //

func handleAdminCallback(c tele.Context, b *tele.Bot, data string) error {
	userID := c.Sender().ID

	if data == "open_owner_panel" || data == "admin_back" {
		c.Delete()
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("📢 Broadcast", "admin_broadcast"), menu.Data("📊 Live Stats", "admin_stats")),
			menu.Row(menu.Data("📡 Manage APIs", "admin_list_apis"), menu.Data("📢 Manage Channels", "admin_manage_channels")),
			menu.Row(menu.Data("🔄 Sync Data", "admin_sync_history"), menu.Data("🔙 Close", "admin_close")),
		)
		return sendMenuWithLogo(c, "🔒 *OWNER CONTROL PANEL*", menu)
	}

	switch {
	case data == "admin_close":
		return c.Delete()

	case data == "admin_broadcast":
		adminStates[userID] = "waiting_for_broadcast"
		return sendMenuWithLogo(c, "📢 *Send the message you want to broadcast.*\n(Text, Photos, and Formatting will be preserved exactly. Type 'cancel' to abort)", nil)

	case data == "admin_sync_history":
		c.Respond(&tele.CallbackResponse{Text: "🔄 Syncing Data..."})
		initializeServerHistory()
		return c.Send("✅ *Sync Complete!*", tele.ModeMarkdown)

	case data == "admin_stats":
		editMenuWithLogo(c, "⏳ *Waiting... Fetching live API stats...*", nil)
		
		dataMutex.RLock()
		totalUsersCount := len(totalUsers)
		dailyUsersCount := len(botStats.DailyUsers)
		totalOTPs := botStats.TotalOTPs
		dataMutex.RUnlock()

		activeServers := 0
		totalServers := len(botConfig)
		statsText := fmt.Sprintf("📊 *ADVANCED BOT STATISTICS*\n━━━━━━━━━━━━━━━━━━\n👥 *Total Users:* `%d`\n📅 *Today's Users:* `%d`\n📩 *Total OTPs Fetched:* `%d`\n *Total Servers:* `%d`\n", totalUsersCount, dailyUsersCount, totalOTPs, totalServers)
		
		statsText += "━━━━━━━━━━━━━━━━━━\n📡 *LIVE API STATUS & NUMBERS:*\n"
		
		totalNumbersOverall := 0
		for _, s := range botConfig {
			if s.Active {
				activeServers++
				apiData, err := fetchAPI(s.APINumbers)
				if err == nil && apiData["aaData"] != nil {
					count := len(apiData["aaData"].([]interface{}))
					statsText += fmt.Sprintf("▪️ *%s:* `%d` numbers ✅\n", s.Name, count)
					totalNumbersOverall += count
				} else {
					statsText += fmt.Sprintf("▪️ *%s:* `Offline/Error ❌`\n", s.Name)
				}
			} else {
				statsText += fmt.Sprintf("▪️ *%s:* `Disabled 🔴`\n", s.Name)
			}
		}
		
		statsText = strings.Replace(statsText, fmt.Sprintf(" *Total Servers:* `%d`\n", totalServers), fmt.Sprintf(" *Total Servers:* `%d`\n🟢 *Active Servers:* `%d`\n", totalServers, activeServers), 1)
		statsText += fmt.Sprintf("━━━━━━━━━━━━━━━━━━\n🔢 *Total Available Numbers:* `%d`\n", totalNumbersOverall)
		
		menu := &tele.ReplyMarkup{}
		menu.Inline(menu.Row(menu.Data("🔙 Back", "admin_back")))
		return editMenuWithLogo(c, statsText, menu)

	case data == "admin_manage_channels":
		menu := &tele.ReplyMarkup{}
		var rows []tele.Row
		for i, ch := range reqChannels {
			rows = append(rows, menu.Row(menu.Data(fmt.Sprintf("❌ Remove %s", ch.Name), fmt.Sprintf("admin_del_chan_%d", i))))
		}
		rows = append(rows, menu.Row(menu.Data("➕ Add New Channel", "admin_add_channel")))
		rows = append(rows, menu.Row(menu.Data("🔙 Back", "admin_back")))
		menu.Inline(rows...)
		return editMenuWithLogo(c, "📢 *Channel Management*", menu)

	case data == "admin_add_channel":
		adminStates[userID] = "waiting_for_channel_target"
		return sendMenuWithLogo(c, "🔗 *Send Channel Link or ID:*\n(Example: @channelname or -100123456)", nil)

	case strings.HasPrefix(data, "admin_del_chan_"):
		idx, _ := strconv.Atoi(strings.Split(data, "_")[3])
		if idx < len(reqChannels) {
			reqChannels = append(reqChannels[:idx], reqChannels[idx+1:]...)
			saveJSON(ChannelsFile, reqChannels)
			c.Respond(&tele.CallbackResponse{Text: "Channel Removed!"})
		}
		return handleAdminCallback(c, b, "admin_manage_channels")

	case data == "admin_list_apis":
		menu := &tele.ReplyMarkup{}
		var rows []tele.Row
		for i, s := range botConfig {
			status := "🔴"
			if s.Active { status = "🟢" }
			rows = append(rows, menu.Row(menu.Data(fmt.Sprintf("%s %s", s.Name, status), fmt.Sprintf("admin_view_srv_%d", i))))
		}
		rows = append(rows, menu.Row(menu.Data("➕ Add New API", "admin_add_api")))
		rows = append(rows, menu.Row(menu.Data("🔙 Back", "admin_back")))
		menu.Inline(rows...)
		return editMenuWithLogo(c, "📋 *Manage APIs*", menu)

	case data == "admin_add_api":
		adminStates[userID] = "waiting_for_api_url"
		return sendMenuWithLogo(c, "📝 *Send the Base API Link here:*\n(Example: `https://.../api/ivs`)", nil)

	case strings.HasPrefix(data, "admin_view_srv_"):
		idx, _ := strconv.Atoi(strings.Split(data, "_")[3])
		if idx >= len(botConfig) { return handleAdminCallback(c, b, "admin_list_apis") }
		server := botConfig[idx]
		
		statusText, btnText := "Disabled ❌", "▶️ Start Server"
		if server.Active { statusText, btnText = "Active ✅", "🛑 Stop Server" }
		
		details := fmt.Sprintf("⚙️ *Server Configuration: %d*\n━━━━━━━━━━━━━━━━━━\n🏷️ *Name:* `%s`\n📡 *Status:* %s\n━━━━━━━━━━━━━━━━━━", idx+1, server.Name, statusText)
		
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data(btnText, fmt.Sprintf("admin_toggle_%d", idx))),
			menu.Row(menu.Data("🗑️ Delete API", fmt.Sprintf("admin_delete_%d", idx))),
			menu.Row(menu.Data("🔙 Back to List", "admin_list_apis")),
		)
		return editMenuWithLogo(c, details, menu)

	case strings.HasPrefix(data, "admin_toggle_"):
		idx, _ := strconv.Atoi(strings.Split(data, "_")[2])
		botConfig[idx].Active = !botConfig[idx].Active
		saveJSON(ServersFile, botConfig)
		c.Respond(&tele.CallbackResponse{Text: "Status Updated!"})
		return handleAdminCallback(c, b, fmt.Sprintf("admin_view_srv_%d", idx))

	case strings.HasPrefix(data, "admin_delete_"):
		idx, _ := strconv.Atoi(strings.Split(data, "_")[2])
		deleteServerHistory(idx)
		botConfig = append(botConfig[:idx], botConfig[idx+1:]...)
		saveJSON(ServersFile, botConfig)
		c.Respond(&tele.CallbackResponse{Text: "API Deleted!"})
		return handleAdminCallback(c, b, "admin_list_apis")
	}

	return c.Respond()
}

// ---------------- USER MENU ---------------- //

func showMainMenu(c tele.Context) error {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	var current []tele.Btn
	for i, s := range botConfig {
		if s.Active {
			current = append(current, menu.Data(fmt.Sprintf(" %s", s.Name), fmt.Sprintf("set_server_%d", i)))
			if len(current) == 2 {
				rows = append(rows, menu.Row(current...))
				current = nil
			}
		}
	}
	if len(current) > 0 { rows = append(rows, menu.Row(current...)) }
	rows = append(rows, menu.Row(menu.Data("🎲 ALL SERVERS (Random)", "country_random_all")))
	if isAdmin(c.Sender().ID) { rows = append(rows, menu.Row(menu.Data("👑 Owner Panel", "open_owner_panel"))) }
	menu.Inline(rows...)
	return editMenuWithLogo(c, "🎛 *Select a Server to Get Number:*", menu)
}

func getCountriesMenu(c tele.Context, sIdx int) error {
	if sIdx >= len(botConfig) { return c.Respond(&tele.CallbackResponse{Text: "❌ Invalid Server", ShowAlert: true}) }
	apiData, err := fetchAPI(botConfig[sIdx].APINumbers)
	if err != nil || apiData["aaData"] == nil { return c.Respond(&tele.CallbackResponse{Text: "❌ Server Offline", ShowAlert: true}) }

	countriesMap := make(map[string]bool)
	for _, item := range apiData["aaData"].([]interface{}) {
		row := item.([]interface{})
		rawName := fmt.Sprintf("%v", row[0])
		if strings.Contains(rawName, "<input") { rawName = fmt.Sprintf("%v", row[1]) }
		countriesMap[fastCleanName(rawName)] = true
	}

	menu := &tele.ReplyMarkup{}
	var btns []tele.Btn
	for country := range countriesMap {
		flag := getFlagFromName(country) // Using the new Flag Library
		btns = append(btns, menu.Data(fmt.Sprintf("%s %s", flag, country), fmt.Sprintf("country_%s", country)))
	}

	var rows []tele.Row
	rows = append(rows, menu.Row(menu.Data("🎲 Random", "country_random")))
	for i := 0; i < len(btns); i += 2 {
		if i+1 < len(btns) { rows = append(rows, menu.Row(btns[i], btns[i+1])) } else { rows = append(rows, menu.Row(btns[i])) }
	}
	rows = append(rows, menu.Row(menu.Data("🔙 Back", "main_menu")))
	menu.Inline(rows...)
	return editMenuWithLogo(c, fmt.Sprintf("🌍 *Select Country (Server: %s)*", botConfig[sIdx].Name), menu)
}

func processNumberFetch(c tele.Context, country string) error {
	dataMutex.RLock()
	selections := userSelections[c.Sender().ID]
	dataMutex.RUnlock()

	if selections == nil || selections["server_idx"] == "" { return showMainMenu(c) }

	sIdx, _ := strconv.Atoi(selections["server_idx"])
	apiData, err := fetchAPI(botConfig[sIdx].APINumbers)

	var validNums [][]string
	if err == nil && apiData["aaData"] != nil {
		for _, item := range apiData["aaData"].([]interface{}) {
			row := item.([]interface{})
			rawName := fmt.Sprintf("%v", row[0])
			numIndex := 2
			if strings.Contains(rawName, "<input") {
				rawName = fmt.Sprintf("%v", row[1])
				numIndex = 3
			}
			cName := fastCleanName(rawName)
			num := fmt.Sprintf("%v", row[numIndex])

			if country == "random" || strings.EqualFold(cName, country) {
				validNums = append(validNums, []string{num, cName})
			}
		}
	}

	if len(validNums) > 0 {
		rand.Seed(time.Now().UnixNano())
		choice := validNums[rand.Intn(len(validNums))]
		dataMutex.Lock()
		userSelections[c.Sender().ID]["num"] = choice[0]
		userSelections[c.Sender().ID]["country"] = choice[1]
		dataMutex.Unlock()

		flag := getFlagFromName(choice[1]) // Added Flag Here too
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("📨 GET OTP", "check_otp")),
			menu.Row(menu.Data("🔄 Refresh Number", fmt.Sprintf("change_number_%s", choice[1]))),
			menu.Row(menu.Data("🔙 Home", "main_menu")),
		)
		text := fmt.Sprintf("📡 *Server:* %s\n🌍 *Country:* %s %s\n📱 *Number:* `+%s`", botConfig[sIdx].Name, choice[1], flag, choice[0])
		return editMenuWithLogo(c, text, menu)
	}
	return c.Respond(&tele.CallbackResponse{Text: "❌ No numbers available.", ShowAlert: true})
}

func checkOTP(c tele.Context) error {
	dataMutex.RLock()
	selections := userSelections[c.Sender().ID]
	dataMutex.RUnlock()

	if selections == nil || selections["num"] == "" {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Session expired.", ShowAlert: true})
	}

	currNum, sIdxStr, cName := selections["num"], selections["server_idx"], selections["country"]
	sIdx, _ := strconv.Atoi(sIdxStr)

	editMenuWithLogo(c, fmt.Sprintf("📱 `+%s`\n⏳ Checking SMS...", currNum), nil)

	hist := loadHistory(sIdx)
	apiData, err := fetchAPI(botConfig[sIdx].APISMS)

	foundMsg, foundSig := "", ""
	if err == nil && apiData["aaData"] != nil {
		for _, item := range apiData["aaData"].([]interface{}) {
			row := item.([]interface{})
			firstCol := fmt.Sprintf("%v", row[0])
			pNum, pMsg := "", ""
			if strings.Contains(firstCol, "<input") || strings.Contains(firstCol, "checkbox") {
				pNum, pMsg = fmt.Sprintf("%v", row[3]), fmt.Sprintf("%v", row[5])
			} else {
				pNum, pMsg = fmt.Sprintf("%v", row[2]), fmt.Sprintf("%v", row[4])
			}
			if strings.Contains(pNum, currNum) || strings.Contains(currNum, pNum) {
				if len(pMsg) > 1 && pMsg != "0" {
					sig := fmt.Sprintf("%s|%s", strings.TrimSpace(pNum), strings.TrimSpace(pMsg))
					if !hist[sig] {
						foundMsg, foundSig = pMsg, sig
						break
					}
				}
			}
		}
	}

	menu := &tele.ReplyMarkup{}
	if foundMsg != "" {
		hist[foundSig] = true
		saveHistory(sIdx, hist)
		dataMutex.Lock()
		botStats.TotalOTPs++
		go saveJSON(StatsFile, botStats)
		dataMutex.Unlock()

		menu.Inline(
			menu.Row(menu.Data("🔄 Change Number", fmt.Sprintf("change_number_%s", cName))),
			menu.Row(menu.Data("🏠 Menu", "main_menu")),
		)
		return editMenuWithLogo(c, fmt.Sprintf("✅ *OTP RECEIVED!*\n📱 `+%s`\n\n💬 `%s`", currNum, foundMsg), menu)
	}

	menu.Inline(
		menu.Row(menu.Data("🔄 TRY AGAIN", "check_otp")),
		menu.Row(menu.Data("🚫 Change", fmt.Sprintf("change_number_%s", cName)), menu.Data("🏠 Menu", "main_menu")),
	)
	return editMenuWithLogo(c, fmt.Sprintf("❌ *OTP NOT FOUND*\n📱 `+%s`\nWait 5s and Try Again.", currNum), menu)
}
