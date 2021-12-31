package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	badger "github.com/dgraph-io/badger/v3"
	"github.com/gagliardetto/solana-go/rpc"
)

const (
	commandPrice  = "price"
	commandVerify = "verify"
)

var (
	discordToken string
	channelId    string
	guildId      string
	roleId       string
	rmCmd        bool
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	flag.StringVar(&discordToken, "discordToken", "", "Discord bot token")
	flag.StringVar(&guildId, "guild", "", "Guild ID")
	flag.StringVar(&channelId, "channel", "", "Daily market report channel ID")
	flag.StringVar(&roleId, "role", "", "OG role ID")
	flag.BoolVar(&rmCmd, "rm", false, "Remove slash commands on shutdown")
	flag.Parse()
}

func main() {
	mintsJson, err := os.ReadFile("the-ones.json")
	if err != nil {
		log.Fatal(err)
	}
	var verifiedMints []string
	if err := json.Unmarshal(mintsJson, &verifiedMints); err != nil {
		log.Fatal(err)
	}

	db, err := badger.Open(badger.DefaultOptions("/tmp/badger/only1/discord"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Panic("failed to create Discord session,", err)
	}
	defer dg.Close()

	handlers := Handlers{
		Conn:          rpc.New("https://only1.genesysgo.net/"),
		Repo:          Repository{Db: db},
		VerifiedMints: verifiedMints,
		GuildId:       guildId,
		ChannelId:     channelId,
		RoleId:        roleId,
	}

	dg.AddHandler(handlers.DailyMarketReport)
	dg.AddHandler(handlers.NftWatchdog)

	commands := []*discordgo.ApplicationCommand{
		{Name: commandPrice, Description: "LIKE token price"},
		{Name: commandVerify, Description: "Verify ownership of The Ones NFT", Options: []*discordgo.ApplicationCommandOption{
			{
				Type:         discordgo.ApplicationCommandOptionString,
				Name:         "public-key",
				Description:  "Your Solana wallet address",
				ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeDM},
				Required:     true,
			},
		}},
	}
	commandHandlers := map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		commandPrice:  handlers.PriceCmd,
		commandVerify: handlers.VerifyCmd,
	}

	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})

	if err = dg.Open(); err != nil {
		log.Panic("failed to open a connection,", err)
		return
	}

	createdCommands, err := dg.ApplicationCommandBulkOverwrite(dg.State.User.ID, "", commands)
	if err != nil {
		log.Panic("failed to overwrite commands", err)
	}

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	log.Print("shutting down...")

	if rmCmd {
		log.Print("removing installed commands...")
		for _, cmd := range createdCommands {
			err := dg.ApplicationCommandDelete(dg.State.User.ID, "", cmd.ID)
			if err != nil {
				log.Fatalf("failed to delete command %q: %v", cmd.Name, err)
			}
		}
	}
}
