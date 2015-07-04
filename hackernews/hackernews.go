package hackernews

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/codegangsta/cli"
	_ "github.com/lib/pq"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	ROOT_URL   = "https://news.ycombinator.com/"
	MAIN_TABLE = "#hnmain tr table tr"
	SPACER     = "spacer"
	TITLE_ROW  = "athing"
	SUBTEXT    = "td.subtext"
)

var number_re = regexp.MustCompile("[0-9]+")

// parseIntFromSelectionText extracts the first series of digits from the
// selection's text. If the text doesn't contain an integer, 0 is returned.
func parseIntFromSelectionText(s *goquery.Selection) int {
	text := number_re.FindString(s.Text())
	i, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return i
}

// The Article struct represents a ranked article from the front page of HN.
type Article struct {
	ID           int
	SnapshotID   int
	Rank         int
	Link         string
	Title        string
	Score        int
	Username     string
	CommentCount int
	CommentsLink string
}

// parseTitleRow sets the rank, link and title of the article.
func (a *Article) parseTitleRow(s *goquery.Selection) {
	a.Rank = parseIntFromSelectionText(s.Find("span.rank"))
	title := s.Find(".title a")
	a.Link, _ = title.Attr("href")
	a.Title = title.Text()
}

// parseSubtextRow sets the Article's score, user, comment count and comments link.
// It returns false if the selection is empty or true if the expected subtext was found.
func (a *Article) parseSubtextRow(s *goquery.Selection) bool {
	if s.Length() == 0 {
		return false
	}
	a.Score = parseIntFromSelectionText(s.Find("span.score"))
	links := s.Find("a")
	a.Username = links.First().Text()
	comments := links.Last()
	a.CommentsLink, _ = comments.Attr("href")
	a.CommentCount = parseIntFromSelectionText(comments)
	return true
}

func (a *Article) store(db *sql.DB) {
	r := db.QueryRow(`INSERT INTO hackernews.articles(rank, link, title, score, username, comment_count, comments_link, snapshot_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id;`,
		a.Rank, a.Link, a.Title, a.Score, a.Username, a.CommentCount, a.CommentsLink, a.SnapshotID)
	err := r.Scan(&a.ID)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("stored article")
}

// The Comment struct represents a user's comment on an article on the front page of Hacker News.
type Comment struct {
	ID        int
	Username  string
	CommentID int
	Color     string
	Content   string
	ArticleID int
	offset    int
}

// parseIndent resolves the width of the visual offset of a comment on HN.
// A comment on the article has an offset of 0, and a reply to a comment has
// an offset greater than the parent's.
func (c *Comment) parseIndent(s *goquery.Selection) {
	width, _ := s.Find("img").Attr("width")
	c.offset, _ = strconv.Atoi(width)
}

// parseCommentHead resolves the commenter's username and the comment's ID.
func (c *Comment) parseCommentHead(s *goquery.Selection) error {
	links := s.Find("a")
	if links.Length() != 2 {
		return errors.New("Wrong number of links in comment head")
	}
	c.Username = links.First().Text()
	_id, _ := links.Next().Attr("href")
	id, err := strconv.Atoi(strings.Split(_id, "=")[1])
	if err != nil {
		return err
	}
	c.ID = id
	return nil
}

// parseCommentBody resolves the color of the comment (downvoted comments are
// displayed with a lighter color) and the content of the comment.
func (c *Comment) parseCommentBody(s *goquery.Selection) {
	color, _ := s.Find("font").Attr("color")
	c.Color = color
	c.Content, _ = s.Html()
}

func (c *Comment) store(db *sql.DB) {
	r := db.QueryRow(`INSERT INTO hackernews.comments (comment_id, username, color, content, article_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id;`,
		c.CommentID, c.Username, c.Color, c.Content, c.ArticleID)
	if err := r.Scan(&c.ID); err != nil {
		log.Fatal(err)
	}
	r = db.QueryRow(`INSERT INTO hackernews.threads (ancestor, descendant, depth)
		VALUES ($1, $1, 0)
		RETURNING id;`,
		c.ID)
	var replyID int
	if err := r.Scan(&replyID); err != nil {
		log.Fatal(err)
	}
	log.Println("stored comment")
}

// addReply embeds a new reply struct in this comment's Replies list.
func (c *Comment) addReply(child *Comment, db *sql.DB) int {
	r := db.QueryRow(`INSERT INTO hackernews.threads (ancestor, descendant, depth)
		SELECT ancestor, $1::integer, depth + 1 FROM hackernews.threads
		WHERE descendant = $2::integer
		RETURNING id, depth;`,
		child.ID, c.ID)
	var id int
	var depth int
	if err := r.Scan(&id, &depth); err != nil {
		log.Fatal(err)
	}
	log.Printf("created reply %d (%d)\n", depth, child.offset)
	return id
}

// The CommentPage struct represents the set of comments on an article on the
// front page of Hacker News.
type CommentPage struct {
	Comments []*Comment
	thread   []*Comment
	depth    int
}

// parse populates the Comments array of a CommentPage with the comments on an article .
func (cp *CommentPage) parse(a *Article, db *sql.DB) *CommentPage {
	// The
	doc, _ := goquery.NewDocument(ROOT_URL + a.CommentsLink)
	comments := doc.Find("span.comment")

	comments.Each(func(_ int, s *goquery.Selection) {
		c := &Comment{ArticleID: a.ID}
		row := s.Parent().Parent()
		c.parseIndent(row.Find("td.ind"))
		c.parseCommentHead(row.Find("span.comhead"))
		// remove boilerplate from the comment text
		s.Find(".reply").Remove()
		c.parseCommentBody(s)
		// prior comments with greater or equal offsets cannot have additional replies
		c.store(db)
		var i int
		for i = cp.depth - 1; i >= 0; i -= 1 {
			if cp.thread[i].offset < c.offset {
				break
			}
		}
		cp.thread = append(cp.thread[:i+1], c)
		cp.depth = len(cp.thread)
		// If there's a prior comment in the thread, this must be a reply to it.
		if cp.depth > 1 {
			cp.thread[cp.depth-2].addReply(c, db)
		}
		cp.Comments = append(cp.Comments, c)
	})
	return cp
}

// The FrontPage represents the articles on front page of hacker news.
type FrontPage struct {
	Articles   []*Article
	SnapshotID int
}

// next initializes a new article and appends it to the front page.
func (f *FrontPage) next() *Article {
	a := &Article{SnapshotID: f.SnapshotID}
	return a
}

// parse gets the latest HN front page and parses the articles listed there
// into Article instances.
func (f *FrontPage) parse(db *sql.DB) *FrontPage {
	page, err := goquery.NewDocument(ROOT_URL)
	if err != nil {
		log.Fatalln(err)
	}
	a := f.next()
	page.Find(MAIN_TABLE).Each(func(_ int, s *goquery.Selection) {
		cls, _ := s.Attr("class")
		switch cls {
		// Articles are separated by empty tr elements with the "spacer" class
		case SPACER:
			a.store(db)
			a = f.next()
			// Each article starts with a title row classed (for better or worse) "athing"
		case TITLE_ROW:
			a.parseTitleRow(s)
			// Title rows are followed by an unclassed tr with a child td classed "subtext"
			// There are also a few other rows without classnames, which should be ignored.
		case "":
			if a.parseSubtextRow(s.Find(SUBTEXT)) {
				f.Articles = append(f.Articles, a)
			}
		}
	})
	return f
}

func (f *FrontPage) store(db *sql.DB) {
	r := db.QueryRow(`INSERT INTO hackernews.snapshots DEFAULT VALUES RETURNING id;`)
	if err := r.Scan(&f.SnapshotID); err != nil {
		log.Fatal(err)
	}
	log.Println("stored front page")
}

func NewFrontPage(db *sql.DB, snapshots chan<- *FrontPage) *FrontPage {
	f := &FrontPage{}
	f.store(db)
	fmt.Println("Snapshotting front page")
	snapshots <- f.parse(db)
	return f
}

func Poll(ctx *cli.Context) {
	db, err := sql.Open("postgres", ctx.String("connection"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// The next snapshot won't be taken until the comments from the previous one
	// have started processing.
	snapshots := make(chan *FrontPage, 1)

	fmt.Println("Starting snapshot loop")

	// Run the first poll right away (don't wait for the first tick).
	NewFrontPage(db, snapshots)

	// Take a snapshot of the front page every interval.
	go func() {
		for _ = range time.Tick(ctx.Duration("interval")) {
			NewFrontPage(db, snapshots)
		}
	}()

	// Don't hammer other websites with requests :)
	limiter := time.Tick(ctx.Duration("throttle"))

	fmt.Println("Starting comments loop")

	// Run until the process is killed.
	for fp := range snapshots {
		fmt.Println("Updating front page comments")
		for _, a := range fp.Articles {
			// Block until the rate limit ticker says we can go.
			<-limiter
			fmt.Printf("Parsing comments for %s\n", a.CommentsLink)
			cp := &CommentPage{}
			cp.parse(a, db)
		}
	}
}
