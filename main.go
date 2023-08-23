package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/corpix/uarand"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
)

// To get api token go in github: Settings > Developer Settings > Personal Access Token > Tokens (Classic) > Generate New Token
const GITHUB_TOKEN = "YourGithubTokenMustBeHere"

func main() {
	// Creating server
	http.HandleFunc("/", imageHandler)

	fmt.Println("Starting...")
	log.Fatal(http.ListenAndServe(":80", nil))
}

// Handle user request
func imageHandler(w http.ResponseWriter, r *http.Request) {
	// When no nickname entered
	if r.URL.Path == "/" {
		w.WriteHeader(http.StatusBadRequest) // 400 status
		w.Write([]byte("Enter github nickname as param, example:\nhttp://" + r.Host + "/mygithubnickname"))
		return
	}

	// Parsing nickname from path
	githubNick := strings.ToLower(strings.Trim(r.URL.Path, "/"))

	success, res := loadResponse(githubNick) // Rate using function

	if success {
		w.WriteHeader(http.StatusNotFound) // 404 status
		if res == -99 {
			w.Write([]byte("Github api limit error"))
		} else {
			w.Write([]byte("Github user not found!"))
		}
	} else {
		// Loading background image
		cont, err := os.ReadFile("Assets/background.png")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError) // 500 status
			w.Write([]byte("Error!"))
		}

		imData, err := png.Decode(bytes.NewReader(cont))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError) // 500 status
			w.Write([]byte("Error!"))
		}

		// Turning background image to *image.RGBA
		rgba := image.NewRGBA(imData.Bounds())
		draw.Draw(rgba, rgba.Bounds(), imData, imData.Bounds().Min, draw.Over)

		// Slicing too long nick
		displayedNick := ""
		if len(githubNick) > 14 {
			displayedNick = githubNick[:14] + "..."
		} else {
			displayedNick = githubNick
		}

		// Text with results
		addText("Rating "+displayedNick+"'s github page:", &rgba, rgba.Bounds().Dx()/2, 325, 56, "Assets/Lato-Bold.ttf")
		addText(fmt.Sprint(res)+"/100", &rgba, rgba.Bounds().Dx()/2, 400, 38, "Assets/Roboto-Regular.ttf")

		// Sending results
		w.WriteHeader(http.StatusOK)
		w.Header().Add("Content-Type", "image/png")
		png.Encode(w, rgba)
	}
}

// Add text to image.RGBA ( I do not know how it is working, I just copy :) )
func addText(text string, rgba **image.RGBA, ax int, ay int, fontsize int, fontname string) {
	fontPath := fontname
	fontBytes, err := os.ReadFile(fontPath)
	if err != nil {
		log.Fatal(err)
	}
	font, err := truetype.Parse(fontBytes)
	if err != nil {
		log.Fatal(err)
	}
	ctx := freetype.NewContext()
	ctx.SetDPI(72)
	ctx.SetFont(font)
	ctx.SetFontSize(float64(fontsize))
	ctx.SetSrc(image.NewUniform(color.Transparent))
	ctx.SetDst(*rgba)
	ctx.SetClip((*rgba).Bounds())

	bounds, _ := ctx.DrawString(text, freetype.Pt(0, 0))

	ctx = freetype.NewContext()
	ctx.SetDPI(72)
	ctx.SetFont(font)
	ctx.SetFontSize(float64(fontsize))
	ctx.SetSrc(image.NewUniform(color.White))
	ctx.SetDst(*rgba)
	ctx.SetClip((*rgba).Bounds())

	x := ax - bounds.X.Round()/2
	y := ay - bounds.Y.Round()/2

	ctx.DrawString(text, freetype.Pt(x, y))
}

// Function for rating
func loadResponse(nickname string) (bool, int64) { // bool - success, int64 - result
	ua := uarand.GetRandom() // Generating user agent

	// Sending request to get user repo
	client := &http.Client{}
	act, err := http.NewRequest("GET", "https://api.github.com/users/"+nickname+"/repos", nil)
	if err != nil {
		return true, -1
	}

	act.Header.Set("User-Agent", ua)
	act.Header.Set("Authorization", "Bearer "+GITHUB_TOKEN)
	resp, err := client.Do(act)

	// To get remaining count of requests of user you can use >> resp.Header.Get("x-ratelimit-remaining")
	// To get rate limit of requests of user you can use >> resp.Header.Get("x-ratelimit-limit")

	if err != nil {
		return true, -1
	}
	if resp.StatusCode != 200 {
		if resp.Header.Get("x-ratelimit-remaining") == "0" {
			return true, -99
		}
		return true, -1
	}

	a, err := io.ReadAll(resp.Body)
	if err != nil {
		return true, -1
	}

	var repoInfo []interface{} // slice with repos info
	err = json.Unmarshal(a, &repoInfo)
	if err != nil {
		return true, -1
	}

	// Sending request to get user info
	act, err = http.NewRequest("GET", "https://api.github.com/users/"+nickname, nil)
	if err != nil {
		return true, -1
	}
	act.Header.Set("User-Agent", ua)
	act.Header.Set("Authorization", "Bearer "+GITHUB_TOKEN)
	resp, err = client.Do(act)

	if err != nil {
		return true, -1
	}
	if resp.StatusCode != 200 {
		if resp.Header.Get("x-ratelimit-remaining") == "0" {
			return true, -99
		}
		return true, -1
	}

	a, err = io.ReadAll(resp.Body)
	if err != nil {
		return true, -1
	}

	var userInfo map[string]interface{} // map with user info
	err = json.Unmarshal(a, &userInfo)
	if err != nil {
		return true, -1
	}

	// Rating
	if len(repoInfo) == 0 {
		return false, 0
	}

	descriptionCount := 0
	topicCount := 0
	watchersCount := 0.0
	hasOutfitRepoPercent := 0

	for _, repos := range repoInfo {
		if a, ok := repos.(map[string]interface{}); ok {
			if desc, ok := a["description"].(string); ok { // Rating repo descriptions
				if len(desc) > 10 {
					descriptionCount++
				}
			}
			if topics, ok := a["topics"].([]interface{}); ok { // Rating repo topics
				if len(topics) > 1 {
					topicCount++
				}
			}
			if watchers, ok := a["watchers"].(float64); ok { // Rating watchers coof
				if watchers >= 10 {
					watchersCount += 1.0
				} else if watchers >= 1 {
					watchersCount += 0.1
				}
			}
			if fullreponame, ok := a["full_name"].(string); ok { // Check exist of special repo
				p := strings.Split(fullreponame, "/")
				if p[0] == p[1] {
					hasOutfitRepoPercent = 100
				}
			}
		} else {
			return true, -1
		}
	}

	followers := 0.0
	bio := 0
	activity := 100
	old := 0
	reposCount := 0.0
	reposGists := 0.0
	blog := 0

	if a, ok := userInfo["followers"].(float64); ok { // Rating followers count
		followers = math.Min(a/25.0, 100.0)
	}
	if a, ok := userInfo["bio"].(string); ok && len(a) > 5 { // Rating user bio
		bio = 100
	}
	if a, ok := userInfo["updated_at"].(string); ok {
		t, err := time.Parse(time.RFC3339, a)
		if err == nil {
			if (time.Hour * 24 * 7).Seconds() < time.Since(t).Seconds() {
				activity = 0
			}
		}
	}
	if a, ok := userInfo["created_at"].(string); ok {
		t, err := time.Parse(time.RFC3339, a)
		if err == nil {
			if (time.Hour * 24 * 365 * 4).Seconds() < time.Since(t).Seconds() {
				old = 100
			} else if (time.Hour * 24 * 365 * 3).Seconds() < time.Since(t).Seconds() {
				old = 75
			} else if (time.Hour * 24 * 365 * 2).Seconds() < time.Since(t).Seconds() {
				old = 50
			} else if (time.Hour * 24 * 365).Seconds() < time.Since(t).Seconds() {
				old = 25
			}
		}
	}
	if a, ok := userInfo["public_repos"].(float64); ok {
		reposCount = math.Min(a/2.0, 100.0)
	}
	if a, ok := userInfo["public_gists"].(float64); ok {
		reposGists = math.Min(a/0.5, 100.0)
	}
	if a, ok := userInfo["blog"].(string); ok {
		if a != "" {
			blog = 100
		}
	}

	descriptionPercent := float64(descriptionCount) / float64(len(repoInfo)) * 100.0
	topicPercent := float64(topicCount) / float64(len(repoInfo)) * 100.0
	watchersPercent := watchersCount / float64(len(repoInfo)) * 100.0

	result := averageWithWeights([]int64{
		int64(descriptionPercent),
		int64(watchersPercent),
		int64(topicPercent),
		int64(hasOutfitRepoPercent),
		int64(followers),
		int64(bio),
		int64(activity),
		int64(old),
		int64(reposCount),
		int64(reposGists),
		int64(blog),
	}, []float64{
		0.5,
		1,
		0.3,
		0.2,
		3,
		0.9,
		0.05,
		0.8,
		0.6,
		0.6,
		0.2,
	})

	return false, result
}

func averageWithWeights(numbers []int64, weights []float64) int64 {
	if len(numbers) != len(weights) {
		log.Fatal("Not same count of numbers and weights!")
	}

	var sumWeight float64 = math.Abs(weights[0])
	var sumNumbersWithWeights int64 = int64(float64(numbers[0]) * math.Abs(weights[0]))

	for index, weight := range weights[1:] {
		sumWeight += math.Abs(weight)
		sumNumbersWithWeights += int64(math.Abs(weight) * float64(numbers[index+1]))
	}

	return int64(float64(sumNumbersWithWeights) / float64(sumWeight))
}
