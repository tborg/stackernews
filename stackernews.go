package main

import (
	"github.com/codegangsta/cli"
	hn "github.com/tborg/stackernews/hackernews"
	"os"
	"time"
)

func main() {
	app := cli.NewApp()
	app.Name = "StackerNews"
	app.Usage = "Tracking and Categorizing Hacker News!"

	app.Commands = []cli.Command{
		{
			Name:   "poll-hn",
			Usage:  "Periodically snapshot the articles on the front page of HN, and their comments.",
			Action: hn.Poll,
			Flags: []cli.Flag{
				cli.DurationFlag{
					Name:  "interval",
					Value: time.Minute,
					Usage: "How frequently to snapshot the front page.",
				},
				cli.DurationFlag{
					Name:  "throttle",
					Value: time.Second,
					Usage: "The comments request frequency cap.",
				},
			},
		},
	}
	app.Run(os.Args)
}
