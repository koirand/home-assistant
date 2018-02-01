package main

import (
	"net/http"
	"net/url"
	"io/ioutil"
	"log"
	"os"
	"fmt"
	"time"
	"encoding/json"
	"github.com/line/line-bot-sdk-go/linebot"
)

type dialogFlowResult struct {
	Id              string     `json:"id"`
	Timestamp       string     `json:"timestamp"`
	Result          struct {
		Parameters  struct {
			Card    string     `json:"Card"`
			Weather string     `json:"Weather"`
		}                      `json:"Parameters"`
		Fulfillment struct{
			Speech  string     `json:"speech"`
		}                      `json:"fulfillment"`
		Score       float64    `json:"score"`
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

				// talkToDialogFlow
				dialogFlowResult, err := talkToDialogFlow(message.Text)
				if err != nil {
					log.Fatal(err)
				}
				if dialogFlowResult.Result.Score >= config.DialogFlow.AcceptScore {
					replyMessage = dialogFlowResult.Result.Fulfillment.Speech
				}

				// Add card to trello
				if dialogFlowResult.Result.Parameters.Card != "" {
					if err := addCardToTrello(dialogFlowResult.Result.Parameters.Card); err !=  nil {
						log.Fatal(err)
					}
				}

				// Push weather forcast
				if dialogFlowResult.Result.Parameters.Weather != "" {
					pushWeatherForcast()
				}

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

			// Reply
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

	bot, err := linebot.New(config.Line.ChannelSecret, config.Line.ChannelAccessToken)
	if err != nil {
		log.Fatal("Faled to create bot instance")
		return
	}

	// Push
	if message != "" {
		log.Println("\x1b[35m[Bot][Text]\x1b[0m ", message)
		if _, err = bot.PushMessage(config.Line.PushTo, linebot.NewTextMessage(message)).Do(); err != nil {
			log.Print(err)
			return
		}
	}
}

func talkToDialogFlow (msg string) (dialogFlowResult, error) {

	var r dialogFlowResult

	// DialogFlow
	values := url.Values{}
	values.Add("v", "20150910")
	values.Add("lang", "ja")
	values.Add("query", msg)
	values.Add("sessionId", "12345")

	// Send request to DialogFlow
	req, err := http.NewRequest("GET", "https://api.dialogflow.com/v1/query" + "?" + values.Encode(), nil)
	if err != nil {
		return r, err
	}

	req.Header.Set("Authorization", config.DialogFlow.Auth)
	client := new(http.Client)
	res, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
		return r, err
	}

	// Parse response
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
		return r, err
	}
	if err := json.Unmarshal(body, &r); err != nil {
		log.Fatal(err)
		return r, err
	}

	return r, nil
}

func addCardToTrello (cardName string) error {

	values := url.Values{}
	values.Add("key", config.Trello.ApiKey)
	values.Add("token", config.Trello.Token)
	values.Add("idList", config.Trello.IdList)
	values.Add("name", cardName)

	_, err := http.PostForm("https://trello.com/1/cards", values)
	if err != nil {
		return err
	}

	return nil
}

type weatherStruct struct {
	List []struct {
		Dt                  string   `json:"dt_txt"`
		Main struct {
			TempMax         float64  `json:"temp_max"`
			TempMin         float64  `json:"temp_min"`
		}                            `json:"main"`
		Weather []struct {
			Description     string   `json:"description"`
			Icon            string   `json:"icon"`
		}                            `json:"weather"`
	}                                `json:"list"`
}

func pushWeatherForcast () {

	baseUrl := "http://api.openweathermap.org/data/2.5/forecast"

	resp, _ := http.Get(baseUrl + "?q=Tokyo,jp&units=metric&appid=" + config.OpenWeatherMap.ApiKey)
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var weatherResult weatherStruct
	if err := json.Unmarshal(body, &weatherResult); err != nil {
		log.Fatal(err)
		return
	}

	// Push
	bot, err := linebot.New(config.Line.ChannelSecret, config.Line.ChannelAccessToken)
	if err != nil {
		log.Println(config.Line.ChannelSecret)
		log.Fatal("Faled to create bot instance")
		return
	}

	log.Println("\x1b[35m[Bot][Weather]\x1b[0m ", weatherResult)

	var dateTime time.Time
	var message linebot.Message
	var carouselColumnList []*linebot.CarouselColumn
	var dateTimeFormat string = "2006-01-02 15:04:05"
	for i := 0; i < 8; i++ {
		dateTime, _ = time.Parse(dateTimeFormat, weatherResult.List[i].Dt)
		dateTime = dateTime.In(time.FixedZone("Asia/Tokyo", 9*60*60))
		carouselColumnList = append(
			carouselColumnList,
			linebot.NewCarouselColumn(
				"https://openweathermap.org/img/w/" + weatherResult.List[i].Weather[0].Icon + ".png",
				dateTime.Format(dateTimeFormat),
				weatherResult.List[i].Weather[0].Description + " " +
					fmt.Sprintf("%f", weatherResult.List[i].Main.TempMax) + "℃ / "+
					fmt.Sprintf("%f", weatherResult.List[i].Main.TempMin) + "℃",
				linebot.NewURITemplateAction("View detail", "https://openweathermap.org/city/1850144"),
			),
		)
	}
	message = linebot.NewTemplateMessage(
		"Today weather forcast",
		linebot.NewCarouselTemplate(carouselColumnList...),
	)

	if _, err := bot.PushMessage(config.Line.PushTo, message).Do(); err != nil {
		log.Print(err)
		return
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
	DialogFlow struct {
		Auth                string  `json:"auth"`
		AcceptScore         float64 `json:"acceptScore"`
	}                               `json:"dialogflow"`
	OpenWeatherMap struct {
		ApiKey              string  `json:"apiKey"`
	}                               `json:"OpenWeatherMap"`
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

