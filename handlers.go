package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dustin/go-humanize"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	only1sdk "github.com/only1nft/solana-sdk-go"
)

const errMsg string = "An unexpected error occurred. Please try again later."

type Handlers struct {
	Conn          *rpc.Client
	Repo          Repository
	VerifiedMints []string
	GuildId       string
	ChannelId     string
	RoleId        string
}

func marketEmbedded(price *CoinGeckoTickerData) *discordgo.MessageEmbed {
	priceUsd := humanize.FormatFloat("", price.Usd)
	usd24hChangePct := humanize.FormatFloat("", price.Usd24hChangePct)
	usdMarketCap := humanize.FormatFloat("", price.UsdMarketCap)

	desc := strings.Join([]string{
		fmt.Sprintf("Current price: %s USD per $LIKE token", priceUsd),
		fmt.Sprintf("24h change: %s%%", usd24hChangePct),
		fmt.Sprintf("Market cap: %s USD", usdMarketCap),
	}, "\n")

	return &discordgo.MessageEmbed{
		Title: "Only1 (LIKE) price now",
		Author: &discordgo.MessageEmbedAuthor{
			Name:    "CoinGecko",
			IconURL: "https://static.coingecko.com/s/thumbnail-007177f3eca19695592f0b8b0eabbdae282b54154e1be912285c9034ea6cbaf2.png",
			URL:     "https://coingecko.com/",
		},
		Color:       9160255, // #8BC63F
		URL:         "https://www.coingecko.com/en/coins/only1",
		Description: desc,
	}
}

func (h *Handlers) DailyMarketReport(s *discordgo.Session, event *discordgo.Ready) {
	for {
		now := time.Now()
		tomorrow := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(time.Hour * 24)
		if diff := tomorrow.Sub(now); diff < 0 {
			time.Sleep(time.Minute)
			continue
		} else {
			time.Sleep(diff)
		}

		if price, err := GetPriceData(); err != nil {
			s.ChannelMessageSendEmbed(h.ChannelId, marketEmbedded(price))
		}
	}
}

// Revoke access if a user doesn't own any other NFTs
func (h *Handlers) revokeAccess(s *discordgo.Session, members []string) (err error) {
	allMints, err := h.Repo.GetAll()
	if err != nil {
		return err
	}
	for _, member := range members {
		d := true
		for _, owner := range allMints {
			if owner.User == member {
				d = false
				break
			}
		}

		if !d {
			continue
		}

		if err = s.GuildMemberRoleRemove(h.GuildId, member, h.RoleId); err != nil {
			log.Printf("failed to revoke a role from user %s: %v", member, err)

			return err
		}

		userChannel, err := s.UserChannelCreate(member)
		if err != nil {
			log.Printf("failed to create user channel: %v", err)

			continue
		}

		if _, err = s.ChannelMessageSend(userChannel.ID, "Your wallet doesn't have The Ones anymore, role has been removed from your account."); err != nil {
			log.Printf("failed to send message: %v", err)
		}
	}

	return nil
}

func (h *Handlers) PriceCmd(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource})
	price, err := GetPriceData()
	if err != nil {
		s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{
			Content: "Failed to retrieve the price. Please try again later.",
		})

		return
	}
	s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{
		Embeds: []*discordgo.MessageEmbed{marketEmbedded(price)},
	})
}

func (h *Handlers) VerifyCmd(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource})
	var recipientID string
	if i.Member != nil {
		recipientID = i.Member.User.ID
	} else {
		recipientID = i.User.ID
	}
	userChannel, err := s.UserChannelCreate(recipientID)
	if err != nil {
		log.Printf("failed to create user channel: %v", err)
		s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: errMsg})
		time.Sleep(time.Minute * 2)
		s.InteractionResponseDelete(s.State.User.ID, i.Interaction)

		return
	}
	if i.Member != nil {
		_, err := s.ChannelMessageSend(userChannel.ID, "Hello! Start typing `/` to verify your OG status.")
		if err != nil {
			s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{
				Content: "Further verification requires the Bot to send you a direct message (DM). Looks like your privacy settings do not allow receiving of direct messages. In order to interact with the Bot, you will have to allow direct messages from server members (*Settings* -> *Privacy & Safety*) and after doing that please DM the Bot by clicking the button below.",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.Button{
								Emoji: discordgo.ComponentEmoji{
									Name: "📥",
								},
								Label: "DM the bot",
								Style: discordgo.LinkButton,
								URL:   fmt.Sprintf("https://discordapp.com/channels/@me/%s/", userChannel.ID),
							},
						},
					},
				},
			})
		} else {
			s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{
				Content: "Further verification requires the Bot to send you a direct message (DM). Only1 bot will message you privately now.",
			})
		}
		time.Sleep(time.Minute * 2)
		s.InteractionResponseDelete(s.State.User.ID, i.Interaction)

		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource})
	input := i.ApplicationCommandData().Options[0].StringValue()

	pk, err := solana.PublicKeyFromBase58(input)
	if err != nil {
		s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{
			Content: fmt.Sprintf("**%s** is not a Solana public key. Try to copy your address from Phantom wallet.", input),
		})

		return
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Minute*3))
	defer cancel()
	var mints []string
	if mints, err = only1sdk.GetOwnedVerifiedMints(ctx, h.Conn, pk, &h.VerifiedMints); err != nil {
		s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: errMsg})

		return
	}

	if len(mints) == 0 {
		s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{
			Content: "Unfortunatelly you don't own any of the NFTs that can be verified.",
		})

		return
	}

	amount := only1sdk.RandSmallAmountOfSol()

	amountUi := float64(amount) / float64(solana.LAMPORTS_PER_SOL)
	s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{
		Content: fmt.Sprintf("The Ones NFT in your wallet: **%d**\n\nNext step: Wallet ownership verification\n\nPlease send **%.9f** **SOL** from **%s** to **%s** (the same wallet) so we can identify you as the owner of the wallet.\n\nIf this operation is not accomplished within 10 minutes, verification process will fail.", len(mints), amountUi, input, input),
	})

	var success bool
	func() {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Minute*10))
		defer cancel()

		for {
			if deadline, _ := ctx.Deadline(); deadline.Unix() < time.Now().Unix() {
				return
			}
			success, _ = only1sdk.FindOwnershipTransfer(ctx, h.Conn, pk, amount)
			if success {
				return
			}
			time.Sleep(time.Second * 3)
		}
	}()

	if !success {
		s.FollowupMessageCreate(s.State.User.ID, i.Interaction, false, &discordgo.WebhookParams{
			Content: "Verification timed out. Try again later.",
		})

		return
	}

	if err = s.GuildMemberRoleAdd(h.GuildId, i.User.ID, h.RoleId); err != nil {
		log.Printf("failed to add the role: %v", err)
		s.FollowupMessageCreate(s.State.User.ID, i.Interaction, false, &discordgo.WebhookParams{Content: errMsg})

		return
	}

	var membersToRemove []string
	for _, mint := range mints {
		if owner, err := h.Repo.Get(mint); err != nil {
			s.FollowupMessageCreate(s.State.User.ID, i.Interaction, false, &discordgo.WebhookParams{Content: errMsg})

			return
		} else if owner != nil && owner.PublicKey != input {
			membersToRemove = append(membersToRemove, owner.PublicKey)
		}
		if err = h.Repo.Set(mint, input, i.User.ID); err != nil {
			s.FollowupMessageCreate(s.State.User.ID, i.Interaction, false, &discordgo.WebhookParams{Content: errMsg})

			return
		}
	}

	if err = h.revokeAccess(s, membersToRemove); err != nil {
		s.FollowupMessageCreate(s.State.User.ID, i.Interaction, false, &discordgo.WebhookParams{Content: errMsg})

		return
	}

	s.FollowupMessageCreate(s.State.User.ID, i.Interaction, false, &discordgo.WebhookParams{Content: "Success. Role has been assigned."})
}

func (h *Handlers) NftWatchdog(s *discordgo.Session, event *discordgo.Ready) {
	for {
		mints, err := h.Repo.GetAll()
		if err != nil {
			time.Sleep(time.Minute * 10)

			continue
		}

		var membersToRemove []string
		for mint, owner := range mints {
			pk, err := only1sdk.GetCurrentNFTOwner(context.Background(), h.Conn, solana.MustPublicKeyFromBase58(mint))
			if err != nil {
				continue
			}
			if pk.String() != owner.PublicKey {
				membersToRemove = append(membersToRemove, owner.User)
				h.Repo.Delete(mint)
			}
		}

		h.revokeAccess(s, membersToRemove)

		time.Sleep(time.Hour * 6)
	}
}
