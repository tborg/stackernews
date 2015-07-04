CREATE SCHEMA hackernews;

CREATE TABLE hackernews.snapshots (
    id SERIAL PRIMARY KEY,
    snapshot_time timestamp with time zone DEFAULT current_timestamp
);

CREATE INDEX hn_snapshot_snapshot_time ON hackernews.snapshots (snapshot_time);

CREATE TABLE hackernews.articles (
    id SERIAL PRIMARY KEY,
    rank integer NOT NULL,
    link text NOT NULL,
    title text NOT NULL,
    score integer NOT NULL,
    username text NOT NULL,
    comment_count integer NOT NULL,
    comments_link text NOT NULL,
    snapshot_id integer NOT NULL REFERENCES hackernews.snapshots (id)
);

CREATE INDEX hn_articles_id_index ON hackernews.articles (id);
CREATE INDEX hn_articles_score_index ON hackernews.articles (score);
CREATE INDEX hn_articles_user_index ON hackernews.articles (username);
CREATE INDEX hn_articles_comments_link_index ON hackernews.articles (comments_link);

CREATE TABLE hackernews.comments (
    id SERIAL PRIMARY KEY,
    comment_id integer NOT NULL,
    username text NOT NULL,
    color text,
    content text NOT NULL,
    article_id integer NOT NULL REFERENCES hackernews.articles (id)
);

CREATE INDEX hn_comments_id_index ON hackernews.comments (id);
CREATE INDEX hn_comments_user_index ON hackernews.comments (username);

CREATE TABLE hackernews.threads (
    id SERIAL PRIMARY KEY,
    ancestor integer NOT NULL REFERENCES hackernews.comments (id),
    descendant integer NOT NULL REFERENCES hackernews.comments (id),
    depth integer NOT NULL
);

CREATE INDEX hn_threads_id ON hackernews.threads (id);
CREATE INDEX hn_threads_ancestor ON hackernews.threads (ancestor);
CREATE INDEX hn_threads_descendant ON hackernews.threads (descendant);
