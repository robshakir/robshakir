// Package main generates a README.md for a GitHub profile.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/oauth2"

	log "github.com/golang/glog"
	"github.com/google/go-github/v35/github"
	"github.com/guptarohit/asciigraph"
)

const (
	ghUsername  string = "robshakir"
	fetchEvents int    = 300
)

func pacific(t time.Time) time.Time {
	pst, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		log.Exitf("LA doesn't exist")
	}
	return t.In(pst)
}

type Events struct {
	Start       time.Time
	Repos       map[string]int
	Actions     map[string]int
	Hours       []float64
	Days        [7]int
	User        *github.User
	Breadcrumbs []*github.Event
}

func NewEvents() *Events {
	return &Events{
		Repos:       map[string]int{},
		Actions:     map[string]int{},
		Hours:       []float64{},
		Days:        [7]int{},
		Breadcrumbs: []*github.Event{},
	}
}

func events(ctx context.Context, client *github.Client) (*Events, error) {
	user, _, err := client.Users.Get(ctx, ghUsername)
	if err != nil {
		return nil, err
	}

	ghevents, _, err := client.Activity.ListEventsPerformedByUser(ctx, ghUsername, false, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		return nil, err
	}

	// Fetch up to 3 pages if needed
	for i := 2; i <= (fetchEvents / 100); i++ {
		e, _, err := client.Activity.ListEventsPerformedByUser(ctx, ghUsername, false, &github.ListOptions{
			Page:    i,
			PerPage: 100,
		})
		if err != nil {
			break
		}
		ghevents = append(ghevents, e...)
	}

	e := NewEvents()
	e.User = user
	hours := map[int]int{}
	for _, event := range ghevents {
		e.Repos[event.GetRepo().GetName()]++
		e.Actions[event.GetType()]++
		t := pacific(*event.CreatedAt)
		hours[t.Hour()]++
		e.Days[int(t.Weekday())]++
	}

	for i := 0; i < 24; i++ {
		e.Hours = append(e.Hours, float64(hours[i]))
	}

	e.Start = pacific(*ghevents[len(ghevents)-1].CreatedAt)
	switch {
	case len(ghevents) >= 10:
		e.Breadcrumbs = ghevents[0:10]
	default:
		e.Breadcrumbs = ghevents
	}

	return e, nil
}

func plotHourOfDay(events *Events) string {
	outBuf := &bytes.Buffer{}
	as := asciigraph.Plot(events.Hours,
		asciigraph.Width(100),
		asciigraph.Height(15),
		asciigraph.Precision(0),
	)
	outBuf.WriteString(as)
	outBuf.WriteString("\n    ")
	width := 100
	interval := 2 // hours between labels
	for i := 0; i <= width; i++ {
		if i%(width/(24/interval)) != 0 {
			outBuf.WriteRune('─')
		} else {
			outBuf.WriteRune('+')
		}
	}
	outBuf.WriteString("\n  ")
	// total width is $width
	//   remaining width is $width - (5 ch * # of intervals)
	//   spaces is therefore remaining / # of intervals
	spaces := (width - 5*(24/interval)) / (24 / interval)
	for i := 0; i <= (24 / interval); i++ {
		outBuf.WriteString(fmt.Sprintf("%02d:00", (i*interval)%24))
		for j := 0; j < spaces; j++ {
			outBuf.WriteRune(' ')
		}
	}
	outBuf.WriteString("\n\n						Commits by Hour of Day\n")

	max := 0.0
	maxHour := 0
	for hour, count := range events.Hours {
		if count > max {
			max = count
			maxHour = hour
		}
	}
	outBuf.WriteString(fmt.Sprintf("\n\nSince %s, I'm most active between %02d:00-%02d:59 - with %.0f events in that hour.\n", events.Start, maxHour, maxHour, max))

	return outBuf.String()
}

func writeActiveRepos(events *Events) string {
	type repoCount struct {
		name  string
		count int
	}
	var repos []repoCount
	max := 0
	for name, count := range events.Repos {
		repos = append(repos, repoCount{name: name, count: count})
		if count > max {
			max = count
		}
	}
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].count > repos[j].count
	})

	outBuf := &bytes.Buffer{}
	for _, r := range repos {
		outBuf.WriteString(fmt.Sprintf("%-40s | ", r.name))
		if max > 0 {
			barLen := (r.count * 40) / max
			for j := 0; j < barLen; j++ {
				outBuf.WriteString("█")
			}
		}
		outBuf.WriteString(fmt.Sprintf(" %d\n", r.count))
	}

	activeRepo := ""
	if len(repos) > 0 {
		activeRepo = repos[0].name
	}

	outBuf.WriteString(fmt.Sprintf("\n\nSince %s, I've been most active in %s, with %d events.\n", events.Start, activeRepo, max))
	return outBuf.String()
}

func plotDayOfWeek(events *Events) string {
	outBuf := &bytes.Buffer{}
	days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

	max := 0
	for _, count := range events.Days {
		if count > max {
			max = count
		}
	}

	for i, count := range events.Days {
		outBuf.WriteString(fmt.Sprintf("%-10s | ", days[i]))
		if max > 0 {
			barLen := (count * 50) / max
			for j := 0; j < barLen; j++ {
				outBuf.WriteString("█")
			}
		}
		outBuf.WriteString(fmt.Sprintf(" %d\n", count))
	}
	return outBuf.String()
}

func writeActivityTypes(events *Events) string {
	outBuf := &bytes.Buffer{}

	type actCount struct {
		name  string
		count int
	}
	var counts []actCount
	max := 0
	for name, count := range events.Actions {
		counts = append(counts, actCount{name: name, count: count})
		if count > max {
			max = count
		}
	}
	sort.Slice(counts, func(i, j int) bool {
		return counts[i].count > counts[j].count
	})

	for _, ac := range counts {
		name := strings.TrimSuffix(ac.name, "Event")
		outBuf.WriteString(fmt.Sprintf("%-20s | ", name))
		if max > 0 {
			barLen := (ac.count * 50) / max
			for j := 0; j < barLen; j++ {
				outBuf.WriteString("█")
			}
		}
		outBuf.WriteString(fmt.Sprintf(" %d\n", ac.count))
	}
	return outBuf.String()
}

func breadcrumbs(events *Events) string {

	actMap := map[string]string{
		"PushEvent":                     "🚢: Pushed some commits to",
		"CommitCommentEvent":            "🗣: Commented on a commit in",
		"CreateEvent":                   "💥: Created a branch in",
		"DeleteEvent":                   "🗑: Deleted a branch in",
		"ForkEvent":                     "🍴: Forked",
		"IssueCommentEvent":             "😃: Commented on an issue in",
		"IssuesEvent":                   "👀: Worked on an issue in",
		"MemberEvent":                   "👉: Prodded at the collaborators for",
		"PublicEvent":                   "🚀: Open sourced some code in",
		"ReleaseEvent":                  "🐿: Created a release in",
		"SponsorshipEvent":              "💰: Sponsored a project in",
		"WatchEvent":                    "⭐️: Starred",
		"PullRequestEvent":              "✍🏼: Created a pull request in",
		"PullRequestReviewEvent":        "🔍: Reviewed a pull request in ",
		"PullRequestReviewCommentEvent": "💬: Commented on a PR in ",
	}

	outBuf := &bytes.Buffer{}
	outBuf.WriteString("### 🍞 Bread Crumbs\n\n")
	for _, e := range events.Breadcrumbs {
		activity := actMap[e.GetType()]
		if activity == "" {
			log.Errorf("activity %s is not mapped to a name", e.GetType())
			continue
		}
		outBuf.WriteString(fmt.Sprintf(" * %s `%s` at %s\n", activity, e.Repo.GetName(), pacific(e.GetCreatedAt())))
	}
	return outBuf.String()
}

func main() {
	flag.Parse()
	ctx := context.Background()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Exitf("null token in environment, did you forget to set GITHUB_TOKEN?")
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	events, err := events(context.Background(), client)
	if err != nil {
		log.Exitf("can't read events, %v", err)
	}

	hours := plotHourOfDay(events)
	repos := writeActiveRepos(events)
	bc := breadcrumbs(events)
	days := plotDayOfWeek(events)
	actions := writeActivityTypes(events)

	outBuf := &bytes.Buffer{}
	outBuf.WriteString(fmt.Sprintf("### 📊 GitHub Stats\n\n"))
	outBuf.WriteString(fmt.Sprintf(" * 👥 **Followers**: %d\n", events.User.GetFollowers()))
	outBuf.WriteString(fmt.Sprintf(" * 👤 **Following**: %d\n", events.User.GetFollowing()))
	outBuf.WriteString(fmt.Sprintf(" * 📦 **Public Repos**: %d\n", events.User.GetPublicRepos()))
	outBuf.WriteString("\n")

	outBuf.WriteString(bc)

	outBuf.WriteString("\n### 🕘 Recent Activity (Last 300 Events)\n")

	outBuf.WriteString("\n#### Hourly Activity\n")
	outBuf.WriteString("```\n")
	outBuf.WriteString(hours)
	outBuf.WriteString("\n```\n")

	outBuf.WriteString("\n#### Weekly Activity\n")
	outBuf.WriteString("```\n")
	outBuf.WriteString(days)
	outBuf.WriteString("\n```\n")

	outBuf.WriteString("\n#### Activity Type Breakdown\n")
	outBuf.WriteString("```\n")
	outBuf.WriteString(actions)
	outBuf.WriteString("\n```\n")

	outBuf.WriteString("\n#### Most Active Repositories\n")
	outBuf.WriteString("```\n")
	outBuf.WriteString(repos)
	outBuf.WriteString("\n```\n")

	outBuf.WriteString("\n---\n")
	outBuf.WriteString("**[robshakir](mailto:robjs@google.com) is not an official Google product.**  \n")
	outBuf.WriteString(fmt.Sprintf("\nLast Updated: %s\n", pacific(time.Now())))

	if err := ioutil.WriteFile("README.md", outBuf.Bytes(), 0644); err != nil {
		log.Exitf("can't write file, %v", err)
	}
}
