CREATE TABLE IF NOT EXISTS election_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS elections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER,
    title TEXT NOT NULL,
    description TEXT,
    start_time DATETIME,
    end_time DATETIME,
    duration_minutes INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (group_id) REFERENCES election_groups(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS candidates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    election_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    position TEXT NOT NULL,
    manifesto TEXT,
    photo_url TEXT,
    vote_count INTEGER DEFAULT 0,
    FOREIGN KEY (election_id) REFERENCES elections(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS votes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    election_id INTEGER NOT NULL,
    candidate_id INTEGER NOT NULL,
    position TEXT NOT NULL,
    voter_hash TEXT NOT NULL,
    voted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(election_id, position, voter_hash),
    FOREIGN KEY (election_id) REFERENCES elections(id) ON DELETE CASCADE,
    FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS admins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    last_login DATETIME
);
