package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"
	"strings"
)

//go:embed template.html
var albumTemplate embed.FS

type albumPost struct {
	Date      string
	Content   string
	Images    []string
	Audio     string
	Comments  []albumComment
	ImageMode string
	Tilt      string
}

type albumComment struct {
	Name    string
	Avatar  template.URL
	Content string
	Date    string
	ReplyTo string
}

type albumYear struct {
	Year  string
	Posts []albumPost
}

type albumFriend struct {
	Name   string
	School string
	Color  string
	Avatar string
	Gone   bool
}

var albumPostTemplate = template.Must(template.New("posts").Funcs(template.FuncMap{"spacedYear": spacedYear}).Parse(`
{{range .}}<section class="year-page" id="year-page-{{.Year}}" data-year-page="{{.Year}}"><div class="year" id="y{{.Year}}" data-year="{{.Year}}">{{spacedYear .Year}}</div><div class="post-grid">
{{range .Posts}}<div class="post" style="--tilt:{{.Tilt}}"><div class="card">
  <span class="date-tag">{{.Date}}</span>
  {{if eq .ImageMode "audio"}}<div class="hifi"><div class="hifi-brand"><b>VOICE ARCHIVE</b><span>录音</span></div><audio controls preload="metadata" src="{{.Audio}}"></audio><p class="hifi-note">{{.Content}}</p></div>
  {{else if eq .ImageMode "none"}}<p class="text-body">{{.Content}}</p>
  {{else if eq .ImageMode "one"}}<figure class="polaroid"><img src="{{index .Images 0}}" alt="动态图片"><figcaption>{{.Content}}</figcaption></figure>
  {{else}}<div class="wall">{{range .Images}}<figure><img src="{{.}}" alt="动态图片"></figure>{{end}}</div><p class="gallery-caption">{{.Content}}</p>{{end}}
  {{if .Comments}}<div class="comments"><div class="comments-head">评论区 <span>{{len .Comments}} 条</span></div>{{range .Comments}}<div class="comment"><img src="{{.Avatar}}" alt=""><div><b>{{.Name}}</b>{{if .ReplyTo}} <small>回复 {{.ReplyTo}}</small>{{end}}<p>{{.Content}}</p><time>{{.Date}}</time></div></div>{{end}}</div>{{end}}
</div></div>
{{end}}</div></section>{{end}}`))

var albumFriendTemplate = template.Must(template.New("friends").Parse(`{{range .Friends}}<div class="medal{{if .Gone}} gone{{end}}" style="{{.Style}}" data-friend-days="{{.FriendDays}}" data-captured-at="{{$.CapturedAt}}">
  {{if .Gone}}<span class="gone-tag">已注销</span><span class="tiny-star">✦</span>{{end}}
  <div class="medal-frame"><img class="medal-avatar" src="{{.Avatar}}" alt="{{.Name}}" loading="lazy"></div><div class="ribbon"><i></i></div>
  <div class="medal-id">{{.Name}}</div><div class="medal-school">{{.School}}{{if .Department}} · {{.Department}}{{end}}</div><div class="friend-days">认识 <span>{{.FriendDays}}</span> 天</div>
  <div class="friend-hover"><b>{{.Name}}</b><span>专业：{{.Major}}</span><span>生日：{{.Birthday}}</span><span>城市：{{.City}}</span><p>{{.Bio}}</p></div>
</div>{{end}}`))

type renderedFriend struct {
	Name, School, Department, Major, Birthday, City, Bio string
	Avatar                                               template.URL
	Style                                                template.CSS
	FriendDays, Gender                                   int
	Gone                                                 bool
}

type friendStats struct {
	Total, Gone, Male, Female int
}

type friendWallView struct {
	Friends    []renderedFriend
	CapturedAt string
	Stats      friendStats
}

type profileTagGroup struct {
	Name string
	Tags []string
}

type profileQuestion struct {
	Content string
	Meta    string
	Images  []string
	Answers []profileAnswer
}

type profileAnswer struct {
	Name, Content, Date string
}

type profileView struct {
	Avatar, Name, ID, Bio string
	Facts                 []string
	TagGroups             []profileTagGroup
	PaperQuestions        []profileQuestion
	BoardQuestions        []profileQuestion
}

var profileTemplate = template.Must(template.New("profile").Parse(`<section class="profile">
  <div class="profile-hero"><div class="profile-photo"><img class="profile-avatar" src="{{.Avatar}}" alt="头像"></div><div>
    <div class="profile-name">{{.Name}}</div><div class="profile-id">{{.ID}}</div>
    <div class="profile-facts">{{range .Facts}}<span class="profile-fact">{{.}}</span>{{end}}</div>
    <p class="profile-bio">{{.Bio}}</p>
  </div></div></div>
  {{if .TagGroups}}<section class="profile-section"><h3>我的标签</h3><div class="tag-groups">{{range .TagGroups}}<div class="tag-group"><b>{{.Name}}</b><div>{{range .Tags}}<span class="profile-tag">{{.}}</span>{{end}}</div></div>{{end}}</div></section>{{end}}
  {{if .PaperQuestions}}<section class="profile-section"><h3>我的交友问卷</h3><div class="question-list">{{range .PaperQuestions}}<div class="question-item">{{.Content}}<small>{{.Meta}}</small></div>{{end}}</div></section>{{end}}
  {{if .BoardQuestions}}<section class="profile-section"><h3>我的黑板墙提问</h3><div class="question-list">{{range .BoardQuestions}}<details class="question-item"><summary>{{.Content}}</summary><small>{{.Meta}}</small>{{if .Images}}<div class="question-images">{{range .Images}}<img src="{{.}}" alt="提问配图" loading="lazy">{{end}}</div>{{end}}{{if .Answers}}<div class="answer-list">{{range .Answers}}<div class="answer"><b>{{.Name}}</b><p>{{.Content}}</p><time>{{.Date}}</time></div>{{end}}</div>{{else}}<p class="no-answer">暂时没有回答</p>{{end}}</details>{{end}}</div></section>{{end}}
</section>`))

func renderAlbum(profile map[string]json.RawMessage, paper map[string]json.RawMessage, questions []json.RawMessage, capturedAt string, normal, blackboard []map[string]json.RawMessage, friends []map[string]json.RawMessage, keepBlackboard, keepFriends bool) ([]byte, error) {
	templateData, err := albumTemplate.ReadFile("template.html")
	if err != nil {
		return nil, fmt.Errorf("读取相册模板失败：%w", err)
	}

	posts := make([]albumPost, 0, len(normal)+len(blackboard))
	for _, memory := range normal {
		post, err := makeAlbumPost(memory, "")
		if err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	if keepBlackboard {
		for _, memory := range blackboard {
			question := rawString(memory, "question")
			answer := rawString(memory, "my_answer")
			post, err := makeAlbumPost(memory, "黑板墙问题："+question+"\n\n我的回答："+answer)
			if err != nil {
				return nil, err
			}
			posts = append(posts, post)
		}
	}

	years := groupPostsByYear(posts)
	var feed bytes.Buffer
	if err := albumPostTemplate.Execute(&feed, years); err != nil {
		return nil, fmt.Errorf("渲染动态相册失败：%w", err)
	}
	data := replaceMarkedBlock(string(templateData), "<!-- FEED_START -->", "<!-- FEED_END -->", feed.String())
	var profileHTML bytes.Buffer
	if err := profileTemplate.Execute(&profileHTML, makeProfileView(profile, paper, questions)); err != nil {
		return nil, fmt.Errorf("渲染个人资料失败：%w", err)
	}
	data = replaceMarkedBlock(data, "<!-- PROFILE_START -->", "<!-- PROFILE_END -->", profileHTML.String())

	renderedFriends := make([]renderedFriend, 0)
	if keepFriends {
		renderedFriends = makeRenderedFriends(friends)
	}
	var friendHTML bytes.Buffer
	friendView := friendWallView{Friends: renderedFriends, CapturedAt: capturedAt, Stats: countFriendStats(renderedFriends)}
	if err := albumFriendTemplate.Execute(&friendHTML, friendView); err != nil {
		return nil, fmt.Errorf("渲染好友勋章失败：%w", err)
	}
	data = replaceMarkedBlock(data, "    <!-- FRIENDS_START -->", "    <!-- FRIENDS_END -->", friendHTML.String())
	stats := friendView.Stats
	data = strings.Replace(data, "共 0 位 · 其中 0 位已远航", fmt.Sprintf("共 %d 位 · 注销/异常 %d 位 · 男 %d 位 · 女 %d 位", stats.Total, stats.Gone, stats.Male, stats.Female), 1)
	return []byte(data), nil
}

func makeProfileView(profile map[string]json.RawMessage, paper map[string]json.RawMessage, boards []json.RawMessage) profileView {
	city := rawString(rawObject(profile["city"]), "name")
	school := rawString(rawObject(profile["school"]), "name")
	department := rawString(rawObject(profile["department"]), "name")
	facts := make([]string, 0, 5)
	for _, value := range []string{rawDisplay(profile, "summer_no"), city, school, department, rawDisplay(profile, "enroll")} {
		if value != "" {
			facts = append(facts, value)
		}
	}
	avatar := rawString(profile, "avatar")
	if avatar == "" {
		avatar = "assets/images/avatar/avatar.png"
	}
	view := profileView{Avatar: avatar, Name: rawString(profile, "nickname"), ID: rawDisplay(profile, "im_id"), Bio: rawString(profile, "bio"), Facts: facts}
	tags := rawObject(profile["tags"])
	for name, rawTags := range tags {
		var values []string
		if json.Unmarshal(rawTags, &values) == nil && len(values) > 0 {
			view.TagGroups = append(view.TagGroups, profileTagGroup{Name: profileTagName(name), Tags: values})
		}
	}
	sort.Slice(view.TagGroups, func(i, j int) bool { return view.TagGroups[i].Name < view.TagGroups[j].Name })
	if paper != nil {
		var paperQuestions []map[string]json.RawMessage
		_ = json.Unmarshal(paper["questions"], &paperQuestions)
		for _, question := range paperQuestions {
			view.PaperQuestions = append(view.PaperQuestions, profileQuestion{Content: rawString(question, "content"), Meta: "交友问卷"})
		}
	}
	for _, rawBoard := range boards {
		var board map[string]json.RawMessage
		if json.Unmarshal(rawBoard, &board) == nil {
			question := profileQuestion{Content: rawString(board, "content"), Meta: fmt.Sprintf("黑板墙 · %s 个回答", rawDisplay(board, "answers_count"))}
			for _, item := range rawMedia(board["images"]) {
				if item.URL != "" && item.Type != "audio" {
					question.Images = append(question.Images, item.URL)
				}
			}
			var answers []map[string]json.RawMessage
			_ = json.Unmarshal(board["answers"], &answers)
			for _, answer := range answers {
				user := rawObject(answer["user"])
				date := rawString(answer, "created_at")
				if len(date) >= 10 {
					date = strings.ReplaceAll(date[:10], "-", ".")
				}
				question.Answers = append(question.Answers, profileAnswer{Name: rawString(user, "nickname"), Content: rawString(answer, "content"), Date: date})
			}
			view.BoardQuestions = append(view.BoardQuestions, question)
		}
	}
	return view
}

func profileTagName(name string) string {
	labels := map[string]string{
		"book": "书籍", "charater": "性格", "hangout": "常去地点", "sport": "运动",
		"music": "音乐", "movie": "电影", "pet": "宠物", "dream": "愿望", "series": "剧集",
		"traval": "旅行", "food": "食物",
	}
	if label, ok := labels[name]; ok {
		return label
	}
	return name
}

func makeAlbumPost(memory map[string]json.RawMessage, contentOverride string) (albumPost, error) {
	content := contentOverride
	if content == "" {
		content = rawString(memory, "content")
	}
	date := rawString(memory, "time")
	if date == "" {
		date = rawString(memory, "created_at")
	}
	if len(date) >= 10 {
		date = strings.ReplaceAll(date[:10], "-", ".")
	}
	media := rawMedia(memory["images"])
	images := make([]string, 0, len(media))
	audio := ""
	for _, item := range media {
		if item.Type == "audio" {
			audio = item.URL
		} else if item.URL != "" {
			images = append(images, item.URL)
		}
	}
	mode := "none"
	if audio != "" {
		mode = "audio"
	} else if len(images) == 1 {
		mode = "one"
	} else if len(images) > 1 {
		mode = "many"
	}
	return albumPost{Date: date, Content: content, Images: images, Audio: audio, Comments: rawComments(memory["comments"]), ImageMode: mode, Tilt: []string{"-1deg", "1.2deg", "-0.8deg", "0.6deg"}[len(content)%4]}, nil
}

func groupPostsByYear(posts []albumPost) []albumYear {
	grouped := map[string][]albumPost{}
	for _, post := range posts {
		year := "未知"
		if len(post.Date) >= 4 {
			year = post.Date[:4]
		}
		grouped[year] = append(grouped[year], post)
	}
	years := make([]string, 0, len(grouped))
	for year := range grouped {
		years = append(years, year)
	}
	sort.Strings(years)
	result := make([]albumYear, 0, len(years))
	for _, year := range years {
		result = append(result, albumYear{Year: year, Posts: grouped[year]})
	}
	return result
}

func makeRenderedFriends(friends []map[string]json.RawMessage) []renderedFriend {
	result := make([]renderedFriend, 0, len(friends))
	for index, friend := range friends {
		info := rawObject(friend["info"])
		if len(info) == 0 {
			info = friend
		}
		name := rawString(friend, "nickname")
		if name == "" {
			name = rawString(info, "nickname")
		}
		if name == "" {
			name = rawString(friend, "name")
		}
		if name == "" {
			name = rawString(info, "name")
		}
		if name == "" {
			name = rawString(friend, "id")
		}
		status := rawString(info, "status")
		avatar := rawString(info, "avatar")
		if avatar == "" {
			avatar = rawString(friend, "avatar")
		}
		if strings.HasPrefix(avatar, "http") {
			avatar = avatarPlaceholder
		}
		if avatar == "" {
			avatar = avatarPlaceholder
		}
		schoolObject := rawObject(info["school"])
		school := rawString(schoolObject, "name")
		if school == "" {
			school = "Summer 好友"
		}
		department := rawString(rawObject(info["department"]), "name")
		gender := rawNumber(info["gender"])
		if gender == 0 {
			gender = rawNumber(friend["gender"])
		}
		gone := status != "" && status != "normal" && status != "OK"
		// html/template 会把内联 style 中的 var() 判为不安全，这里直接用等值十六进制色。
		color := "#6ec6ff"
		if gender == 2 {
			color = "#ff8fc7"
		}
		if gone {
			color = "#4a5fb5"
			if gender == 2 {
				color = "#b28dff"
			}
		}
		friendDays := rawNumber(info["friend_days"])
		if friendDays == 0 {
			friendDays = rawNumber(friend["friend_days"])
		}
		style := template.CSS(fmt.Sprintf("--d:%.2fs;--mc:%s", float64(index)*0.04, color))
		result = append(result, renderedFriend{
			Name: name, School: school, Department: department, Major: rawString(info, "major"),
			Birthday: rawString(info, "birthday"), City: rawString(rawObject(info["city"]), "name"), Bio: rawString(info, "bio"),
			Style: style, Avatar: template.URL(avatar), FriendDays: friendDays, Gender: gender, Gone: gone,
		})
	}
	return result
}

func countFriendStats(friends []renderedFriend) friendStats {
	stats := friendStats{Total: len(friends)}
	for _, friend := range friends {
		if friend.Gone {
			stats.Gone++
		}
		if friend.Gender == 1 {
			stats.Male++
		} else if friend.Gender == 2 {
			stats.Female++
		}
	}
	return stats
}

func replaceMarkedBlock(source, start, end, replacement string) string {
	startIndex := strings.Index(source, start)
	if startIndex < 0 {
		return source
	}
	endIndex := strings.Index(source[startIndex+len(start):], end)
	if endIndex < 0 {
		return source
	}
	endIndex += startIndex + len(start)
	return source[:startIndex] + replacement + source[endIndex+len(end):]
}

func rawString(object map[string]json.RawMessage, key string) string {
	if object == nil {
		return ""
	}
	var value string
	if json.Unmarshal(object[key], &value) == nil {
		return value
	}
	return ""
}

func rawDisplay(object map[string]json.RawMessage, key string) string {
	if object == nil {
		return ""
	}
	var number json.Number
	if json.Unmarshal(object[key], &number) == nil && number.String() != "" {
		return number.String()
	}
	return rawString(object, key)
}

func rawObject(raw json.RawMessage) map[string]json.RawMessage {
	var value map[string]json.RawMessage
	_ = json.Unmarshal(raw, &value)
	return value
}

// avatarPlaceholder 是无头像时的内联占位图，保证离线可用。
const avatarPlaceholder = "data:image/svg+xml;charset=utf-8,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Cdefs%3E%3Cpattern id='p' width='16' height='16' patternUnits='userSpaceOnUse' patternTransform='rotate(45)'%3E%3Crect width='16' height='16' fill='%23fdf6e3'/%3E%3Crect width='8' height='16' fill='%236ec6ff'/%3E%3C/pattern%3E%3C/defs%3E%3Ccircle cx='32' cy='32' r='32' fill='url(%23p)'/%3E%3C/svg%3E"

func orAvatarPlaceholder(value string) string {
	if value == "" || strings.HasPrefix(value, "http") {
		return avatarPlaceholder
	}
	return value
}

func rawStrings(raw json.RawMessage) []string {
	var values []string
	if json.Unmarshal(raw, &values) == nil {
		return values
	}
	return nil
}

type albumMedia struct {
	URL  string
	Type string
}

func rawMedia(raw json.RawMessage) []albumMedia {
	var objects []struct {
		URL  string `json:"url"`
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &objects) == nil {
		result := make([]albumMedia, 0, len(objects))
		for _, object := range objects {
			result = append(result, albumMedia{URL: object.URL, Type: object.Type})
		}
		return result
	}
	var stringsValue []string
	if json.Unmarshal(raw, &stringsValue) == nil {
		result := make([]albumMedia, 0, len(stringsValue))
		for _, value := range stringsValue {
			result = append(result, albumMedia{URL: value})
		}
		return result
	}
	return nil
}

func rawComments(raw json.RawMessage) []albumComment {
	var comments []map[string]json.RawMessage
	if json.Unmarshal(raw, &comments) != nil {
		return nil
	}
	result := make([]albumComment, 0, len(comments))
	for _, comment := range comments {
		user := rawObject(comment["user"])
		toUser := rawObject(comment["to_user"])
		date := rawString(comment, "created_at")
		if len(date) >= 10 {
			date = strings.ReplaceAll(date[:10], "-", ".")
		}
		result = append(result, albumComment{Name: rawString(user, "nickname"), Avatar: template.URL(orAvatarPlaceholder(rawString(user, "avatar"))), Content: rawString(comment, "content"), Date: date, ReplyTo: rawString(toUser, "nickname")})
	}
	return result
}

func spacedYear(year string) string {
	return strings.Join(strings.Split(year, ""), " ")
}
