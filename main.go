package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dustin/go-humanize"
)

const CoinGeckoUrl = "https://api.coingecko.com/api/v3/simple/price?ids=only1&vs_currencies=usd&include_market_cap=true&include_24hr_vol=true&include_24hr_change=true"

var (
	Token     string
	ChannelId string

	commands = []*discordgo.ApplicationCommand{
		{Name: "price", Description: "LIKE token price"},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"price": sendPriceMessage,
	}
)

type CoinGeckoTickerData struct {
	Usd             float64 `json:"usd"`
	UsdMarketCap    float64 `json:"usd_market_cap"`
	Usd24hVolume    float64 `json:"usd_24h_vol"`
	Usd24hChangePct float64 `json:"usd_24h_change"`
}

type CoinGeckoResponse struct {
	Only1 CoinGeckoTickerData `json:"only1"`
}

func getPriceData() CoinGeckoTickerData {
	resp, err := http.Get(CoinGeckoUrl)
	if err != nil {
		log.Println("failed to retrieve coingecko price", err)
		time.Sleep(time.Second * 30)
		return getPriceData()
	}

	var parsed CoinGeckoResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		log.Println("failed to parse coingecko response", err)
		time.Sleep(time.Second * 30)
		return getPriceData()
	}

	return parsed.Only1
}

func marketEmbedded(price CoinGeckoTickerData) *discordgo.MessageEmbed {
	priceUsd := humanize.FormatFloat("", price.Usd)
	usd24hChangePct := humanize.FormatFloat("", price.Usd24hChangePct)
	usdMarketCap := humanize.FormatFloat("", price.UsdMarketCap)

	desc := []string{
		fmt.Sprintf("Current price: %s USD per $LIKE token", priceUsd),
		fmt.Sprintf("24h change: %s%%", usd24hChangePct),
		fmt.Sprintf("Market cap: %s USD", usdMarketCap),
	}

	return &discordgo.MessageEmbed{
		Title: "Only1 (LIKE) price now",
		Author: &discordgo.MessageEmbedAuthor{
			Name:    "CoinGecko",
			IconURL: "https://static.coingecko.com/s/thumbnail-007177f3eca19695592f0b8b0eabbdae282b54154e1be912285c9034ea6cbaf2.png",
			URL:     "https://coingecko.com/",
		},
		Color:       9160255, // #8BC63F
		URL:         "https://www.coingecko.com/en/coins/only1",
		Description: strings.Join(desc, "\n"),
	}
}

func sendPriceMessage(s *discordgo.Session, i *discordgo.InteractionCreate) {
	price := getPriceData()
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{marketEmbedded(price)},
		},
	})
}

func dailyMarketReport(s *discordgo.Session, event *discordgo.Ready) {
	for {
		now := time.Now()
		tomorrow := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(time.Hour * 24)
		if diff := tomorrow.Sub(now); diff < 0 {
			time.Sleep(time.Minute)
			continue
		} else {
			time.Sleep(diff)
		}

		price := getPriceData()
		s.ChannelMessageSendEmbed(ChannelId, marketEmbedded(price))
	}
}

func init() {
	flag.StringVar(&Token, "token", "", "Discord bot token")
	flag.StringVar(&ChannelId, "channelId", "", "Daily market report channel ID")
	flag.Parse()
}

func main() {
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		log.Panic("failed to create Discord session,", err)
	}

	dg.AddHandler(dailyMarketReport)
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})

	if err = dg.Open(); err != nil {
		log.Panic("failed to open a connection,", err)
		return
	}

	for _, guild := range dg.State.Guilds {
		if _, err := dg.ApplicationCommandBulkOverwrite(dg.State.User.ID, guild.ID, commands); err != nil {
			log.Panic("failed to overwrite commands", err)
		}
	}

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	fmt.Println("Shutting down...")

	dg.Close()
}
