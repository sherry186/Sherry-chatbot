// Copyright 2016 LINE Corporation
//
// LINE Corporation licenses this file to you under the Apache License,
// version 2.0 (the "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at:
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/line/line-bot-sdk-go/v7/linebot"
)

func main() {
	godotenv.Load()
	app, err := NewKitchenSink(
		os.Getenv("CHANNEL_SECRET"),
		os.Getenv("CHANNEL_TOKEN"),
		os.Getenv("APP_BASE_URL"),
	)
	if err != nil {
		log.Fatal(err)
	}

	// create rich menu
	richMenu := linebot.RichMenu{
		Size:        linebot.RichMenuSize{Width: 2500, Height: 1686},
		Selected:    true,
		Name:        "Menu1",
		ChatBarText: "Learn More",
		Areas: []linebot.AreaDetail{
			{
				Bounds: linebot.RichMenuBounds{X: 0, Y: 0, Width: 2500, Height: 843},
				Action: linebot.RichMenuAction{
					Type: linebot.RichMenuActionTypeMessage,
					Text: "作品集",
				},
			},
			{
				Bounds: linebot.RichMenuBounds{X: 0, Y: 843, Width: 833, Height: 843},
				Action: linebot.RichMenuAction{
					Type: linebot.RichMenuActionTypeURI,
					URI:  "https://github.com/sherry186",
					Text: "Github",
				},
			},
			{
				Bounds: linebot.RichMenuBounds{X: 833, Y: 843, Width: 834, Height: 843},
				Action: linebot.RichMenuAction{
					Type: linebot.RichMenuActionTypeMessage,
					Text: "履歷",
				},
			},
			{
				Bounds: linebot.RichMenuBounds{X: 1667, Y: 843, Width: 833, Height: 843},
				Action: linebot.RichMenuAction{
					Type: linebot.RichMenuActionTypeMessage,
					Text: "個人介紹",
				},
			},
		},
	}
	res, err := app.bot.CreateRichMenu(richMenu).Do()
	if err != nil {
		log.Fatal(err)
	}
	log.Println(res.RichMenuID)

	// upload richmenu
	if _, err = app.bot.UploadRichMenuImage(res.RichMenuID, "./static/richmenu/richmenu.jpg").Do(); err != nil {
		log.Fatal(err)
	}

	// set default rich menu
	if _, err = app.bot.SetDefaultRichMenu(res.RichMenuID).Do(); err != nil {
		log.Fatal(err)
	}

	// serve /static/** files
	staticFileServer := http.FileServer(http.Dir("static"))
	http.HandleFunc("/static/", http.StripPrefix("/static/", staticFileServer).ServeHTTP)
	// serve /downloaded/** files
	downloadedFileServer := http.FileServer(http.Dir(app.downloadDir))
	http.HandleFunc("/downloaded/", http.StripPrefix("/downloaded/", downloadedFileServer).ServeHTTP)

	http.HandleFunc("/callback", app.Callback)
	// This is just a sample code.
	// For actually use, you must support HTTPS by using `ListenAndServeTLS`, reverse proxy or etc.
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), nil); err != nil {
		log.Fatal(err)
	}
}

// KitchenSink app
type KitchenSink struct {
	bot         *linebot.Client
	appBaseURL  string
	downloadDir string
}

// NewKitchenSink function
func NewKitchenSink(channelSecret, channelToken, appBaseURL string) (*KitchenSink, error) {
	apiEndpointBase := os.Getenv("ENDPOINT_BASE")
	if apiEndpointBase == "" {
		apiEndpointBase = linebot.APIEndpointBase
	}
	bot, err := linebot.New(
		channelSecret,
		channelToken,
		linebot.WithEndpointBase(apiEndpointBase), // Usually you omit this.
	)
	if err != nil {
		return nil, err
	}
	downloadDir := filepath.Join(filepath.Dir(os.Args[0]), "line-bot")
	_, err = os.Stat(downloadDir)
	if err != nil {
		if err := os.Mkdir(downloadDir, 0777); err != nil {
			return nil, err
		}
	}
	return &KitchenSink{
		bot:         bot,
		appBaseURL:  appBaseURL,
		downloadDir: downloadDir,
	}, nil
}

// Callback function for http server
func (app *KitchenSink) Callback(w http.ResponseWriter, r *http.Request) {
	events, err := app.bot.ParseRequest(r)
	if err != nil {
		if err == linebot.ErrInvalidSignature {
			w.WriteHeader(400)
		} else {
			w.WriteHeader(500)
		}
		return
	}
	for _, event := range events {
		log.Printf("Got event %v", event)
		switch event.Type {
		case linebot.EventTypeMessage:
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				if err := app.handleText(message, event.ReplyToken, event.Source); err != nil {
					log.Print(err)
				}
			case *linebot.ImageMessage:
				if err := app.handleImage(message, event.ReplyToken); err != nil {
					log.Print(err)
				}
			case *linebot.FileMessage:
				if err := app.handleFile(message, event.ReplyToken); err != nil {
					log.Print(err)
				}
			case *linebot.LocationMessage:
				if err := app.handleLocation(message, event.ReplyToken); err != nil {
					log.Print(err)
				}
			case *linebot.StickerMessage:
				if err := app.handleSticker(message, event.ReplyToken); err != nil {
					log.Print(err)
				}
			default:
				log.Printf("Unknown message: %v", message)
			}
		case linebot.EventTypeFollow:
			// if err := app.replyText(event.ReplyToken, "Got followed event"); err != nil {
			// 	log.Print(err)
			// }
		case linebot.EventTypeUnfollow:
			log.Printf("Unfollowed this bot: %v", event)
		case linebot.EventTypeJoin:
			if err := app.replyText(event.ReplyToken, "Joined "+string(event.Source.Type)); err != nil {
				log.Print(err)
			}
		case linebot.EventTypeLeave:
			log.Printf("Left: %v", event)
		case linebot.EventTypePostback:
			data := event.Postback.Data
			if data == "DATE" || data == "TIME" || data == "DATETIME" {
				data += fmt.Sprintf("(%v)", *event.Postback.Params)
			}

			if data == "Dormy" {
				if err := app.replyText(event.ReplyToken, `🌟專案簡介
Dormy 你的宿舍好幫手是一個媒合住宿需求與願意提供協助方的任務媒合平台。需求方可以透過平台刊登任務，供給方則能透過平台查看所有刊登中的任務，並針對能提供協助的任務發起應徵，等待需求方接受應徵。

🌟重點項目
· 利用 Python FastAPI開發後端 RestfulAPI
· 使用SQLAlchemy ORM 技術串聯 PostgreSQL
· 設計關聯式資料庫 DB Schema

🌟使用技術
Python, Sql ORM, Restful api`); err != nil {
					log.Print(err)
				}
			}

			if data == "Pathfinder" {
				if err := app.replyText(event.ReplyToken, `🌟專案簡介
學習歷程為108課綱下，高中生升學準備的重要項目之一，旨在讓高中生記錄三年在學表現，並減輕學生在高中整理備審資料的負擔。然而，在經過調查問卷（799筆樣本）、深入訪談（14位受訪者）後，我們發現：「77.8%的同學不知道要怎麼規劃學習歷程檔案」，同學們在書寫學習歷程的總體規劃有關鍵問題尚待解決。為解決上述問題，我們設計了 Pathfinder，一個專屬於高中生學習歷程、生涯探索的App。立基於ColleGo!網站資料上，Pathfinder串接三大功能：紀錄面板、儀表分析板、探索活動板，以個人化推薦、整合性的功能為關鍵特色，旨在為使用者打造個別專屬的生涯探索之旅。

🌟重點項目
· 榮獲2021大專校院資訊應用服務創新競賽 資訊應用組 第二名
· 入圍 2021 Reimagine Education Awards （華頓商院舉辦，錄取率 12%）
· 設計系統架構，包含選定 MongoDB 後端資料庫、GraphQL API 以及 React Native 製做前端 App
· 管理 Kanban 與 Scrum meeting，定期舉行 review meeting 統整團隊進度

🌟使用技術
React native, MongoDB, GraphQL`); err != nil {
					log.Print(err)
				}
			}

			if data == "全球營運系統智能化" {
				if err := app.replyText(event.ReplyToken, `🌟專案簡介
此專案與全球快遞公司合作，解決公司騎士資源調度問題

🌟重點項目
· 利用 Python sklearn 套件跑複回歸模型分析(lasso, ridge) ，抓取重要變數
· 利用 Python matplotlib 套件進行敘述統計

🌟使用技術
Python sklearn, matplotlib, Lasso and Ridge regression`); err != nil {
					log.Print(err)
				}
			}

			if data == "Classification" {
				if err := app.replyText(event.ReplyToken, `🌟專案簡介
分析 2019-2021 的股票漲跌趨勢，建立分類模型進行漲跌預測。

🌟重點項目
· 利用 Python Sklearn 套件跑 Naive Bayes, Decision Tree, Random Forest, KNN, SVM 等分類器模型，挑出準確率最高者進行新聞漲停及跌停分類
· 根據 Precision, Recall 及 F1 等指標，針對不同風險接受程度投資者客製化分類模型

🌟使用技術
Python sklearn, random forest, svm, knn, naive bayes`); err != nil {
					log.Print(err)
				}
			}

			if data == "Docker" {
				if err := app.replyText(event.ReplyToken, `🌟專案簡介
利用 docker 技術搭建 client 及 server 的 container 環境完成連線

🌟重點項目
· 利用官方 Python Image建立 client 及 server 的 docker container
· 使用 docker volume 將 server 傳送結果掛載至實體檔案路徑
· 使用 docker compose 同時開啟 client 及 server 的服務

🌟使用技術
Docker`); err != nil {
					log.Print(err)
				}
			}

		case linebot.EventTypeBeacon:
			if err := app.replyText(event.ReplyToken, "Got beacon: "+event.Beacon.Hwid); err != nil {
				log.Print(err)
			}
		default:
			log.Printf("Unknown event: %v", event)
		}
	}
}

func (app *KitchenSink) handleText(message *linebot.TextMessage, replyToken string, source *linebot.EventSource) error {
	switch message.Text {
	case "profile":
		if source.UserID != "" {
			profile, err := app.bot.GetProfile(source.UserID).Do()
			if err != nil {
				return app.replyText(replyToken, err.Error())
			}
			if _, err := app.bot.ReplyMessage(
				replyToken,
				linebot.NewTextMessage("Display name: "+profile.DisplayName),
				linebot.NewTextMessage("Status message: "+profile.StatusMessage),
			).Do(); err != nil {
				return err
			}
		} else {
			return app.replyText(replyToken, "Bot can't use profile API without user ID")
		}
	case "個人介紹":
		imageURL := app.appBaseURL + "/static/buttons/avatar.jpg"
		template := linebot.NewButtonsTemplate(
			imageURL, "關於 Sherry", "大家好，我是 Sherry 葉小漓，目前就讀台大資管系大三，未來希望能當一名軟體工程師。請多指教！",
			linebot.NewMessageAction("了解更多", "了解更多"),
			linebot.NewMessageAction("Sherry 的電話", "Sherry 的電話"),
			linebot.NewMessageAction("Sherry 的 email", "Sherry 的 email"),
			linebot.NewURIAction("Sherry 的 facebook", "https://www.facebook.com/hsiaoli.yeh.1/"),
		)
		if _, err := app.bot.ReplyMessage(
			replyToken,
			linebot.NewTemplateMessage("about me", template),
		).Do(); err != nil {
			return err
		}
	case "履歷":
		page1URL := app.appBaseURL + "/static/resume/sherry_resume_page1.jpg"
		page2URL := app.appBaseURL + "/static/resume/sherry_resume_page2.jpg"
		template := linebot.NewImageCarouselTemplate(
			linebot.NewImageCarouselColumn(
				page1URL,
				linebot.NewURIAction("Google Drive", "https://drive.google.com/file/d/1S-czYOxV9Nce8rMGPStbrOh2n48AhIlE/view?usp=sharing"),
			),
			linebot.NewImageCarouselColumn(
				page2URL,
				linebot.NewURIAction("Google Drive", "https://drive.google.com/file/d/1S-czYOxV9Nce8rMGPStbrOh2n48AhIlE/view?usp=sharing"),
			),
		)
		if _, err := app.bot.ReplyMessage(
			replyToken,
			linebot.NewTemplateMessage("resume", template),
		).Do(); err != nil {
			return err
		}

		// page1URL := app.appBaseURL + "/static/resume/sherry_resume_page1.jpg"
		// page2URL := app.appBaseURL + "/static/resume/sherry_resume_page2.jpg"
		// if _, err := app.bot.ReplyMessage(
		// 	replyToken,
		// 	linebot.NewImageMessage(page1URL, page1URL),
		// 	linebot.NewImageMessage(page2URL, page2URL),
		// ).Do(); err != nil {
		// 	return err
		// }
	case "作品集":
		imageURLDormy := app.appBaseURL + "/static/projects/dormy.png"
		imageURLPathfinder := app.appBaseURL + "/static/projects/pathfinder.png"
		imageURLGlobal := app.appBaseURL + "/static/projects/globalDelivery.png"
		imageURLFinance := app.appBaseURL + "/static/projects/finance.png"
		imageURLDocker := app.appBaseURL + "/static/projects/docker.jpg"
		template := linebot.NewCarouselTemplate(
			linebot.NewCarouselColumn(
				imageURLDormy, "Dormy 你的宿舍生活好幫手", "Dormy 你的宿舍好幫手是一個媒合住宿需求與願意提供協助方的任務媒合平台。",
				linebot.NewPostbackAction("作品介紹", "Dormy", "", "Dormy 作品介紹"),
				linebot.NewURIAction("github 連結", "https://github.com/sherry186/Dorm_Service"),
			),
			linebot.NewCarouselColumn(
				imageURLPathfinder, "Pathfinder 與您探索無限可能", " Pathfinder是一個專屬於高中生紀錄學習歷程與進行生涯探索的App。",
				linebot.NewPostbackAction("作品介紹", "Pathfinder", "", "Pathfinder 作品介紹"),
				linebot.NewURIAction("github 連結", "https://github.com/sherry186/Pathfinder"),
			),
			linebot.NewCarouselColumn(
				imageURLDocker, "DOCKER PROJECT: CLIENT-SERVER連線", "利用 Docker 技術建立 client server 連線。",
				linebot.NewPostbackAction("作品介紹", "Docker", "", "DOCKER CLIENT-SERVER連線 作品介紹"),
				linebot.NewURIAction("github 連結", "https://github.com/sherry186/Distributed-Systems-Container-Practice"),
			),
			linebot.NewCarouselColumn(
				imageURLGlobal, "產學合作 - 營運系統智能化模型建置", "與全球快遞公司合作，解決公司騎士資源調度問題。",
				linebot.NewPostbackAction("作品介紹", "全球營運系統智能化", "", "全球營運系統智能化 專案介紹"),
				linebot.NewURIAction("尚無連結，導引至個人 github", "https://github.com/sherry186"),
			),
			linebot.NewCarouselColumn(
				imageURLFinance, "財經新聞漲停及跌停文件分類", "分析 2019-2021 的股票漲跌趨勢，建立分類模型進行漲跌預測。",
				linebot.NewPostbackAction("作品介紹", "Classification", "", "財經新聞漲停及跌停文件分類 作品介紹"),
				linebot.NewURIAction("尚無連結，導引至個人 github", "https://github.com/sherry186"),
			),
		)
		if _, err := app.bot.ReplyMessage(
			replyToken,
			linebot.NewTemplateMessage("side projects", template),
		).Do(); err != nil {
			return err
		}
	case "Sherry 的電話":
		if err := app.replyText(replyToken, "0909100476"); err != nil {
			log.Print(err)
		}

	case "Sherry 的 email":
		if err := app.replyText(replyToken, "hsiaoliy@gmail.com"); err != nil {
			log.Print(err)
		}

	case "了解更多":
		if err := app.replyText(replyToken, `我是葉小漓，來自新北市。父親是香港中文大學財金系教授，因此十歲以前就讀香港的國際學校。高中時就讀北一女中數理資優班，當時跟著台大資訊系廖世偉教授作區塊鏈相關的專題研究，過程中設計出一個以區塊鏈為底層技術的藥品供應鏈系統，並獲得台北市科展優等獎。

大學就讀台大資管系，修習系上資料結構與演算法、作業系統、資料庫管理、網路技術與應用等必修課程，以及分散式系統、大數據與商業分析等選修課程。豐富的修課經驗使我累積堅實的資訊底子，也培養我的程式能力以及實作經驗。我接觸過的程式語言有 Python、Javascript、C++、Go，曾用 Python 架設後端系統以及進行數據分析、訓練模型以及使用Javascript 及 React.js 建立前端系統等等。另外，也有運用過 Docker 技術打造 container 環境、並且操作過 aws 平台。對我而言，程式是一個達成目標的工具，若有需求，就努力學習並運用。

課程之餘，我也有在新創公司擔任前端工程師，熟悉 git 版本控制流程、以及學習與多個不同角色溝協作。而在此過程中也發現，比起做切版及畫面顯示等工作，我對於系統的穩定性與安全性設計更感興趣。

未來在工作上，我希望能夠發揮自己的技術專長，為社會貢獻一己之力。「取之於社會、回饋於社會」是我認同的理念，期許自己在未來能夠發揮正面的影響力！
`); err != nil {
			log.Print(err)
		}
	case "bye":
		switch source.Type {
		case linebot.EventSourceTypeUser:
			return app.replyText(replyToken, "Bot can't leave from 1:1 chat")
		case linebot.EventSourceTypeGroup:
			if err := app.replyText(replyToken, "Leaving group"); err != nil {
				return err
			}
			if _, err := app.bot.LeaveGroup(source.GroupID).Do(); err != nil {
				return app.replyText(replyToken, err.Error())
			}
		case linebot.EventSourceTypeRoom:
			if err := app.replyText(replyToken, "Leaving room"); err != nil {
				return err
			}
			if _, err := app.bot.LeaveRoom(source.RoomID).Do(); err != nil {
				return app.replyText(replyToken, err.Error())
			}
		}
	}
	return nil
}

func (app *KitchenSink) handleImage(message *linebot.ImageMessage, replyToken string) error {
	return app.handleHeavyContent(message.ID, func(originalContent *os.File) error {
		// You need to install ImageMagick.
		// And you should consider about security and scalability.
		previewImagePath := originalContent.Name() + "-preview"
		_, err := exec.Command("convert", "-resize", "240x", "jpeg:"+originalContent.Name(), "jpeg:"+previewImagePath).Output()
		if err != nil {
			return err
		}

		originalContentURL := app.appBaseURL + "/downloaded/" + filepath.Base(originalContent.Name())
		previewImageURL := app.appBaseURL + "/downloaded/" + filepath.Base(previewImagePath)
		if _, err := app.bot.ReplyMessage(
			replyToken,
			linebot.NewImageMessage(originalContentURL, previewImageURL),
		).Do(); err != nil {
			return err
		}
		return nil
	})
}

func (app *KitchenSink) handleFile(message *linebot.FileMessage, replyToken string) error {
	return app.replyText(replyToken, fmt.Sprintf("File `%s` (%d bytes) received.", message.FileName, message.FileSize))
}

func (app *KitchenSink) handleLocation(message *linebot.LocationMessage, replyToken string) error {
	if _, err := app.bot.ReplyMessage(
		replyToken,
		linebot.NewLocationMessage(message.Title, message.Address, message.Latitude, message.Longitude),
	).Do(); err != nil {
		return err
	}
	return nil
}

func (app *KitchenSink) handleSticker(message *linebot.StickerMessage, replyToken string) error {
	if _, err := app.bot.ReplyMessage(
		replyToken,
		linebot.NewStickerMessage(message.PackageID, message.StickerID),
	).Do(); err != nil {
		return err
	}
	return nil
}

func (app *KitchenSink) replyText(replyToken, text string) error {
	if _, err := app.bot.ReplyMessage(
		replyToken,
		linebot.NewTextMessage(text),
	).Do(); err != nil {
		return err
	}
	return nil
}

func (app *KitchenSink) handleHeavyContent(messageID string, callback func(*os.File) error) error {
	content, err := app.bot.GetMessageContent(messageID).Do()
	if err != nil {
		return err
	}
	defer content.Content.Close()
	log.Printf("Got file: %s", content.ContentType)
	originalContent, err := app.saveContent(content.Content)
	if err != nil {
		return err
	}
	return callback(originalContent)
}

func (app *KitchenSink) saveContent(content io.ReadCloser) (*os.File, error) {
	file, err := ioutil.TempFile(app.downloadDir, "")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	_, err = io.Copy(file, content)
	if err != nil {
		return nil, err
	}
	log.Printf("Saved %s", file.Name())
	return file, nil
}
