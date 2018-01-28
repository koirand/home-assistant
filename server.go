package main

import (
	"net/http"
	"net/url"
	"io/ioutil"
	"log"
	"os"
	"encoding/json"
	"github.com/line/line-bot-sdk-go/linebot"
)

type DialogflowResult struct {
	Id              string     `json:"id"`
	Timestamp       string     `json:"timestamp"`
	Result          struct {
		Parameters  struct {
			Card    string     `json:"Card"`
		}                      `json:"Parameters"`
		Fulfillment struct{
			Speech  string     `json:"speech"`
		}                      `json:"fulfillment"`
	}                          `json:"result"`
}

func lineWebhookHandler(w http.ResponseWriter, r *http.Request) {

	//Create line bot
	bot, err := linebot.New(config.Line.ChannelSecret, config.Line.ChannelAccessToken)
	if err != nil {
		log.Fatal("Faled to create bot instance")
		return
	}

	//Parse request from line
	events, err := bot.ParseRequest(r)
	if err != nil {
		if err == linebot.ErrInvalidSignature {
			w.WriteHeader(400)
		} else {
			w.WriteHeader(500)
		}
		return
	}

	//Reply message
	for _, event := range events {

		if event.Type == linebot.EventTypeMessage {

			var replyMessage string
			switch message := event.Message.(type) {

			// Text message
			case *linebot.TextMessage:
				log.Println("\x1b[32m[User][Text]\x1b[0m ", message.Text)

				// Dialogflow
				values := url.Values{}
				values.Add("v", "20150910")
				values.Add("lang", "ja")
				values.Add("query", message.Text)
				values.Add("sessionId", "12345")

				// Send request to dialogflow
				req, err := http.NewRequest("GET", "https://api.dialogflow.com/v1/query" + "?" + values.Encode(), nil)
				if err != nil {
					log.Fatal(err)
					return
				}
				req.Header.Set("Authorization", config.Dialogflow.Auth)
				client := new(http.Client)
				res, err := client.Do(req)
				if err != nil {
					log.Fatal(err)
					return
				}

				// Parse response
				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					log.Fatal(err)
					return
				}
				var dialogFlowResult DialogflowResult
				if err := json.Unmarshal(body, &dialogFlowResult); err != nil {
					log.Fatal(err)
					return
				}

				// Add card to trello
				if dialogFlowResult.Result.Parameters.Card != "" {
					values = url.Values{}
					values.Add("key", config.Trello.ApiKey)
					values.Add("token", config.Trello.Token)
					values.Add("idList", config.Trello.IdList)
					values.Add("name", dialogFlowResult.Result.Parameters.Card)

					_, err := http.PostForm("https://trello.com/1/cards", values)
					if err != nil {
						log.Fatal(err)
						return
					}

				}
				replyMessage = dialogFlowResult.Result.Fulfillment.Speech

			// Sticker message
			case *linebot.StickerMessage:
				log.Println("\x1b[32m[User][Sticker]\x1b[0m ", message.PackageID, message.StickerID )
				switch {
				// Flog sticker
				case message.PackageID == "2000002" && message.StickerID == "48473" :
					replyMessage = config.ReplyMessageToStamp.ID_2000002_48473
				case message.PackageID == "2000002" && message.StickerID == "48436" :
					replyMessage = config.ReplyMessageToStamp.ID_2000002_48436
				}
			}

			// Reply message
			if replyMessage != "" {
				log.Println("\x1b[35m[Bot][Text]\x1b[0m ", replyMessage)
				if _, err = bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage(replyMessage)).Do(); err != nil {
					log.Print(err)
					return
				}
			}
		}
	}
}

func linePushHandler(w http.ResponseWriter, r *http.Request) {

	message := r.FormValue("message")

	channelSecret := os.Getenv("CHANNEL_SECRET")
	channelAccessToken := os.Getenv("CHANNEL_ACCESS_TOKEN")

	bot, err := linebot.New(channelSecret, channelAccessToken)
	if err != nil {
		log.Fatal("Faled to create bot instance")
		return
	}

	// Push message
	var toGroupID = config.Line.PushTo
	if message != "" {
		log.Println("\x1b[35m[Bot][Text]\x1b[0m ", message)
		if _, err = bot.PushMessage(toGroupID, linebot.NewTextMessage(message)).Do(); err != nil {
			log.Print(err)
			return
		}
	}
}

type configStruct struct {
    Port                    string  `json:"port"`
	Line struct {
		ChannelSecret       string  `json:"channelSecret"`
		ChannelAccessToken  string  `json:"channelAccessToken"`
		PushTo              string  `json:"pushTo"`
	}                               `json:"line"`
	Trello struct {
		ApiKey              string  `json:"apiKey"`
		Token               string  `json:"token"`
		IdList              string  `json:"idList"`
	}                               `json:"trello"`
	Dialogflow struct {
		Auth                string  `json:"auth"`
	}                               `json:"dialogflow"`
	SshCredential struct {
		FullChainPath       string  `json:"fullChainPath"`
		PrivateKeyPath      string  `json:"privateKeyPath"`
	}                               `json:"sshCredential"`
	ReplyMessageToStamp struct {
		ID_2000002_48473    string  `json:"ID_2000002_48473"`
		ID_2000002_48436    string  `json:"ID_2000002_48436"`
	}                               `json:"replyMessageToStamp"`
}

var config configStruct

func newConfig(filePath string) (configStruct, error) {

	var c configStruct
	jsonBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return c, err
	}

	if err := json.Unmarshal(jsonBytes, &c); err != nil {
		return c, err
	}

	//default value
	if c.Port == "" {
		c.Port = "9090"
	}

    return c, nil
}

func main() {

	//load config
	var err error
	if len(os.Args) == 2  {
		config, err = newConfig(os.Args[1])
	} else {
		config, err = newConfig("config.json")
	}
    if err != nil {
        log.Fatal(err)
		return
    }

	http.HandleFunc("/lineWebhook", lineWebhookHandler)
	http.HandleFunc("/linePush", linePushHandler)

	log.Println("Listening " + config.Port + " port.....")
	err = http.ListenAndServeTLS(":" + config.Port, config.SshCredential.FullChainPath, config.SshCredential.PrivateKeyPath, nil)
	//err := http.ListenAndServe(":" + config.Port, nil)
	if err != nil {
		log.Print(err)
	}
}

