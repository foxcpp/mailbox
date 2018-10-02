package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"

	eimap "github.com/emersion/go-imap"
	"github.com/foxcpp/mailbox/proto/common"
	"github.com/foxcpp/mailbox/proto/imap"
	_ "github.com/mattn/go-sqlite3"
)

// Returned if requested information is not cached in DB.
var ErrNullValue = errors.New("cachedb: null value encountered")

// TODO: Consider using ATTACH DATABASE to have one connection for multiple accounts.

/*
Simple wrapper around SQLite-based cache.

Yes, we use SQLite for cache. It's simple, reliable and flexible solution.

Database schema:

dirinfo:
Various information about directories. Currently only UIDVALIDITY value.

Columns:
- dir (string) [index]
- uidvalidity (int, nullable)
- unreadcount (int, nullable)

meta:
Various meta-information extracted from headers.

Indexes:
- dir + uid
Columns:
- dir (string)
  Directory, where message stored.
- uid (int)
  Message UID.
- timestamp (int, unix timestamp)
  Date header.
- sender (string)
  From header.
- recipients (comma-separated string)
  To header.
- cc (comma-separated string)
  CC header.
- bcc (comma-separated string)
  BCC header.
- messageid (string)
  Message-Id header.
- replyto (string)
  Reply-To header.
- subject (string)
  Subject header.
- hdrs (blob)

tags table simply stores information about message tags (flags), one row for message-tag pair.
Indexes:
- dir + uid
Columns:
- dir (string)
  Dirctory name, where corresponing message stored.
- uid (int)
  UID of corresponding message.
- tag (string)
  Tag name.

parts table stores information about each message body part (as defined by proto
abstraction level).
Indexes:
- dir + uid + indx
Columns:
- dir (string)
  Dirctory name, where corresponing message stored.
- uid (int)
  UID of corresponding message.
- indx (int)
  Sequence number of part in message. 1-based.
- attachment (int)
  1 if message part considered as an attachment. 0 otherwise.
  Body is never cached for attachments.
- content_type (string)
  MIME type of part.
- content_subtype (string)
  MIME subtype of part.
- content_type_params (string)
- size (int)
  Size of body in bytes.
- filename (string)
  Name of attached file (if attachment is 1).
- hdrs (blob)
  MIME-headers blob containing all other headers.
- body (blob, nullable)
  Body without MIME-header.
*/
type CacheDB struct {
	d *sql.DB

	// List directories.
	dirList *sql.Stmt

	// Set/Get UIDVALIDITY value for dir.
	uidValidity    *sql.Stmt
	setUidValidity *sql.Stmt

	// Set/Get unread count value for dir.
	unreadCount    *sql.Stmt
	setUnreadCount *sql.Stmt

	isMsglistValid    *sql.Stmt
	invalidateDirinfo *sql.Stmt
	invalidateMsglist *sql.Stmt

	// Add new directory to list.
	addDir *sql.Stmt

	// Remove directory from list.
	remDir *sql.Stmt

	// Remove all messages from specified directory.
	remDirMsgs  *sql.Stmt
	remDirMsgs2 *sql.Stmt
	remDirMsgs3 *sql.Stmt

	// Get message meta-data using dir + UID.
	getAllMsgs *sql.Stmt

	// Get message headers blob using dir + UID.
	getMsgHdrs *sql.Stmt

	resolveUid *sql.Stmt

	// Get message meta-data + all headers using sequence number + dir.
	getMsgBySeq *sql.Stmt

	// Get message meta-data + all headers using UID + dir.
	getMsgByUid *sql.Stmt

	// Get message tags using UID + dir.
	getMsgTags *sql.Stmt

	// Get information about message parts by UID + dir.
	getMsgPartInfo *sql.Stmt

	// Get message part using UID and part index + dir.
	getMsgPart *sql.Stmt

	// Remove message meta-information.
	delMsg *sql.Stmt

	// Remove message parts.
	delMsgParts *sql.Stmt

	// Remove all tags from msg.
	delMsgTags *sql.Stmt

	// Add tag to message.
	addTag *sql.Stmt
	// Remove tag from message.
	remTag *sql.Stmt

	// Insert part, replace if already present.
	addPart *sql.Stmt

	// Insert message meta-info, replace if already present.
	addMsg *sql.Stmt
}

type Dirwrapper struct {
	parent *CacheDB
	dir    string
}

func OpenCacheDB(path string) (*CacheDB, error) {
	db := new(CacheDB)
	var err error
	db.d, err = sql.Open("sqlite3", "file:"+path+"?cache=shared&_journal=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	if err := db.initSchema(); err != nil {
		return nil, err
	}
	return db, db.initStmts()
}

func (db *CacheDB) SQL() *sql.DB {
	return db.d
}

func (db *CacheDB) Close() error {
	return db.d.Close()
}

func (db *CacheDB) initSchema() error {
	db.d.Exec(`PRAGMA foreign_keys = ON`)
	db.d.Exec(`PRAGMA auto_vacuum = INCREMENTAL`)
	db.d.Exec(`PRAGMA journal_mode = WAL`)
	db.d.Exec(`PRAGMA locking_mode = EXLUSIVE`)
	db.d.Exec(`PRAGMA defer_foreign_keys = ON`)
	db.d.Exec(`PRAGMA synchronous = NORMAL`)
	db.d.Exec(`PRAGMA temp_store = MEMORY`)
	db.d.Exec(`PRAGMA cache_size = 5000`)
	db.d.Exec(`PRAGMA optimize`)

	_, err := db.d.Exec(`
		CREATE TABLE IF NOT EXISTS dirinfo (
			dir TEXT PRIMARY KEY NOT NULL,
			uidvalidity INT DEFAULT NULL,
			unreadcount INT DEFAULT NULL,
			msglistvalid INT DEFAULT 0
		)`)
	if err != nil {
		return err
	}
	_, err = db.d.Exec(`
		CREATE TABLE IF NOT EXISTS meta (
			dir TEXT NOT NULL,
			uid INT NOT NULL,
			timestamp INT DEFAULT "",
			sender TEXT DEFAULT "",
			recipients TEXT DEFAULT "",
			cc TEXT DEFAULT "",
			bcc TEXT DEFAULT "",
			messageid TEXT DEFAULT "",
			replyto TEXT DEFAULT "",
			subject TEXT DEFAULT "",
			hdrs BLOB DEFAULT NULL,
			PRIMARY KEY (dir, uid),
			FOREIGN KEY (dir) REFERENCES dirinfo(dir)
		)`)

	_, err = db.d.Exec(`
		CREATE TABLE IF NOT EXISTS tags (
			dir TEXT NOT NULL,
			uid INT NOT NULL,
			tag TEXT NOT NULL,
			PRIMARY KEY (dir, uid, tag),
			FOREIGN KEY (dir, uid) REFERENCES meta(dir, uid)
		)`)
	if err != nil {
		return err
	}

	_, err = db.d.Exec(`
		CREATE TABLE IF NOT EXISTS parts (
			dir TEXT NOT NULL,
			uid INT NOT NULL,
			indx INT NOT NULL,
			attachment INT NOT NULL DEFAULT 0,
			content_type TEXT NOT NULL DEFAULT "text",
			content_subtype TEXT NOT NULL DEFAULT "plain",
			content_type_params TEXT NOT NULL DEFAULT "",
			size INT NOT NULL,
			filename TEXT NOT NULL DEFAULT "",
			hdrs BLOB NOT NULL,
			body BLOB DEFAULT NULL,
			PRIMARY KEY (dir, uid, indx),
			FOREIGN KEY (dir, uid) REFERENCES meta(dir, uid)
		)`)
	return err
}

func (db *CacheDB) initStmts() error {
	var err error
	db.dirList, err = db.d.Prepare(`SELECT dir FROM dirinfo`)
	if err != nil {
		return err
	}

	db.uidValidity, err = db.d.Prepare(`SELECT uidvalidity FROM dirinfo WHERE dir = ?`)
	if err != nil {
		return err
	}

	db.setUidValidity, err = db.d.Prepare(`UPDATE dirinfo SET uidvalidity = ? WHERE dir = ?`)
	if err != nil {
		return err
	}

	db.unreadCount, err = db.d.Prepare(`SELECT unreadcount FROM dirinfo WHERE dir = ?`)
	if err != nil {
		return err
	}

	db.setUnreadCount, err = db.d.Prepare(`UPDATE dirinfo SET unreadcount = ? WHERE dir = ?`)
	if err != nil {
		return err
	}

	db.isMsglistValid, err = db.d.Prepare(`SELECT msglistvalid FROM dirinfo WHERE dir = ?`)
	if err != nil {
		return err
	}

	db.invalidateDirinfo, err = db.d.Prepare(`UPDATE dirinfo SET unreadcount = NULL, uidvalidity = NULL, msglistvalid = 0 WHERE dir = ?`)
	if err != nil {
		return err
	}

	db.invalidateMsglist, err = db.d.Prepare(`UPDATE dirinfo SET msglistvalid = 0 WHERE dir = ?`)
	if err != nil {
		return err
	}

	db.addDir, err = db.d.Prepare(`INSERT OR IGNORE INTO dirinfo(dir) VALUES (?)`)
	if err != nil {
		return err
	}

	db.remDir, err = db.d.Prepare(`DELETE FROM dirinfo WHERE dir = ?`)
	if err != nil {
		return err
	}

	db.remDirMsgs, err = db.d.Prepare(`
		DELETE FROM parts
		WHERE uid IN (SELECT uid FROM meta WHERE dir = ?) AND dir = ?`)
	if err != nil {
		return err
	}
	db.remDirMsgs2, err = db.d.Prepare(`DELETE FROM meta WHERE dir = ?`)
	if err != nil {
		return err
	}
	db.remDirMsgs3, err = db.d.Prepare(`DELETE FROM tags WHERE dir = ?`)
	if err != nil {
		return err
	}

	db.getAllMsgs, err = db.d.Prepare(`
		SELECT uid,timestamp,sender,recipients,cc,bcc,messageid,replyto,subject,hdrs
		FROM meta WHERE dir = ?`)
	if err != nil {
		return err
	}
	db.getMsgHdrs, err = db.d.Prepare(`SELECT hdrs FROM meta WHERE dir = ? AND uid = ?`)
	if err != nil {
		return err
	}

	db.resolveUid, err = db.d.Prepare(`SELECT uid FROM meta WHERE dir = ? LIMIT 1 OFFSET ?-1`)
	if err != nil {
		return err
	}

	db.getMsgBySeq, err = db.d.Prepare(`
		SELECT uid,timestamp,sender,recipients,cc,bcc,messageid,replyto,subject,hdrs
		FROM meta WHERE dir = ? LIMIT 1 OFFSET ?-1`)
	if err != nil {
		return err
	}
	db.getMsgByUid, err = db.d.Prepare(`
		SELECT uid,timestamp,sender,recipients,cc,bcc,messageid,replyto,subject,hdrs
		FROM meta
		WHERE dir = ? AND uid = ?`)
	if err != nil {
		return err
	}

	db.getMsgTags, err = db.d.Prepare(`SELECT tag FROM tags WHERE dir = ? AND uid = ?`)
	if err != nil {
		return err
	}

	db.getMsgPartInfo, err = db.d.Prepare(`
		SELECT attachment,content_type,content_subtype,content_type_params,size,filename,hdrs
		FROM parts
		WHERE dir = ? AND uid = ?`)
	if err != nil {
		return err
	}

	db.getMsgPart, err = db.d.Prepare(`
		SELECT body FROM parts
		WHERE dir = ? AND uid = ? AND indx = ?`)
	if err != nil {
		return err
	}

	db.delMsg, err = db.d.Prepare(`DELETE FROM meta WHERE dir = ? AND uid = ?`)
	if err != nil {
		return err
	}

	db.delMsgParts, err = db.d.Prepare(`DELETE FROM parts WHERE dir = ? AND uid = ?`)
	if err != nil {
		return err
	}

	db.delMsgTags, err = db.d.Prepare(`DELETE FROM tags WHERE dir = ? AND uid = ?`)
	if err != nil {
		return err
	}

	db.addTag, err = db.d.Prepare(`INSERT OR REPLACE INTO tags VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}

	db.remTag, err = db.d.Prepare(`DELETE FROM tags WHERE dir = ? AND uid = ? AND tag = ?`)
	if err != nil {
		return err
	}

	db.addPart, err = db.d.Prepare(`INSERT OR IGNORE INTO parts VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}

	db.addMsg, err = db.d.Prepare(`
		INSERT OR REPLACE
		INTO meta
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}

	return nil
}

func (db *CacheDB) DirList() ([]string, error) {
	rows, err := db.dirList.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dirs := []string{}
	for rows.Next() {
		dir := ""
		if err = rows.Scan(&dir); err != nil {
			return nil, err
		}
		dirs = append(dirs, dir)
	}

	return dirs, nil
}

func (db *CacheDB) AddDir(name string) error {
	_, err := db.addDir.Exec(name)
	return err
}

// Remove directory from CacheDB.
// Note: Child directories are NOT removed.
func (db *CacheDB) RemoveDir(name string) error {
	tx, err := db.d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Stmt(db.remDir).Exec(name)
	if err != nil {
		return err
	}
	_, err = tx.Stmt(db.remDirMsgs).Exec(name, name)
	if err != nil {
		return err
	}
	_, err = tx.Stmt(db.remDirMsgs2).Exec(name)
	if err != nil {
		return err
	}
	_, err = tx.Stmt(db.remDirMsgs3).Exec(name)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (db *CacheDB) RenameDir(old string, new string) error {
	tx, err := db.d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE dirinfo SET dir = ? WHERE dir = ?`, old, new); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE meta SET dir = ? WHERE dir = ?`, old, new); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE tags SET dir = ? WHERE dir = ?`, old, new); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE parts SET dir = ? WHERE dir = ?`, old, new); err != nil {
		return err
	}

	return tx.Commit()
}

func (db *CacheDB) Dir(name string) *Dirwrapper {
	return &Dirwrapper{db, name}
}

func (d *Dirwrapper) UidValidity() (uint32, error) {
	row := d.parent.uidValidity.QueryRow(d.dir)
	value := sql.NullInt64{}
	if err := row.Scan(&value); err != nil {
		return 0, err
	}
	if !value.Valid {
		return uint32(value.Int64), ErrNullValue
	}
	return uint32(value.Int64), nil
}

// UpdateMsglist adds new messages from newList while preserving all extra information about old messages not present in newList (i.e. this is not just replace).
// For example, cache may contain text body but newList entry does not. Text body will be preserved.
// Note: This function assumes that UIDVALIDITY value is same for newList and old one in cache.
// Note 2: Currently, all part information (including bodies) from newList is ignored if message is already present in cache.
func (d *Dirwrapper) UpdateMsglist(newList []imap.MessageInfo) error {
	tx, err := d.parent.d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	oldUids := make(map[uint32]bool)
	rows, err := d.parent.d.Query(`SELECT uid FROM meta WHERE dir = ?`, d.dir)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		uid := uint32(0)
		if err := rows.Scan(&uid); err != nil {
			return err
		}
		oldUids[uid] = true
	}
	for _, msg := range newList {
		if !oldUids[msg.UID] {
			if err := d.addMsg(tx, &msg); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (d *Dirwrapper) MarkAsValid() error {
	_, err := d.parent.d.Exec(`UPDATE dirinfo SET msglistvalid = 1 WHERE dir = ?`, d.dir)
	return err
}

func (d *Dirwrapper) SetUidValidity(newVal uint32) error {
	_, err := d.parent.setUidValidity.Exec(newVal, d.dir)
	return err
}

func (d *Dirwrapper) UnreadCount() (uint, error) {
	row := d.parent.unreadCount.QueryRow(d.dir)
	value := sql.NullInt64{}
	if err := row.Scan(&value); err != nil {
		return 0, err
	}
	if !value.Valid {
		return 0, ErrNullValue
	}
	return uint(value.Int64), nil
}

func (d *Dirwrapper) SetUnreadCount(newVal uint) error {
	_, err := d.parent.setUnreadCount.Exec(d.dir, newVal)
	return err
}

func (d *Dirwrapper) ListMsgs() ([]imap.MessageInfo, error) {
	tx, err := d.parent.d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	validity := uint(0)
	row := tx.Stmt(d.parent.isMsglistValid).QueryRow(d.dir)
	if err := row.Scan(&validity); err != nil {
		return nil, err
	}
	if validity != 1 {
		return nil, ErrNullValue
	}

	rows, err := tx.Stmt(d.parent.getAllMsgs).Query(d.dir)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := []imap.MessageInfo{}
	for rows.Next() {
		msg, err := readMessageInfo(rows)
		if err != nil {
			return nil, err
		}

		rows, err := tx.Stmt(d.parent.getMsgTags).Query(d.dir, msg.UID)
		if err != nil {
			return nil, err
		}
		if err := readTagList(msg, rows); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		rows, err = tx.Stmt(d.parent.getMsgPartInfo).Query(d.dir, msg.UID)
		if err != nil {
			return nil, err
		}
		if err := readPartInfo(msg, rows); err != nil {
			return nil, err
		}
		rows.Close()

		res = append(res, *msg)
	}
	return res, tx.Commit()
}

func (d *Dirwrapper) GetMsgHdrs(uid uint32) (common.Header, error) {
	row := d.parent.getMsgHdrs.QueryRow(d.dir, uid)
	hdrs := []byte{}
	if err := row.Scan(hdrs); err != nil {
		return nil, err
	}
	return common.ReadHeader(hdrs)
}

func (d *Dirwrapper) GetMsgBySeq(seq uint32) (*imap.MessageInfo, error) {
	row := d.parent.getMsgBySeq.QueryRow(d.dir, seq)

	msg, err := readMessageInfo(row)
	if err != nil {
		return nil, err
	}

	rows, err := d.parent.getMsgTags.Query(d.dir, msg.UID)
	if err != nil {
		return nil, err
	}
	if err := readTagList(msg, rows); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = d.parent.getMsgPartInfo.Query(d.dir, msg.UID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if err := readPartInfo(msg, rows); err != nil {
		return nil, err
	}
	return msg, nil
}

func convAddrList(l []*mail.Address) []common.Address {
	res := make([]common.Address, len(l))
	for i, a := range l {
		res[i] = common.Address(*a)
	}
	return res
}

type Scannable interface {
	Scan(dest ...interface{}) error
}

func readMessageInfo(r Scannable) (*imap.MessageInfo, error) {
	uid, timestamp, sender, recipientsStr, ccStr := uint32(0), int64(0), "", "", ""
	bccStr, messageId, replyTo, subject, hdrs := "", "", "", "", []byte{}
	err := r.Scan(&uid, &timestamp, &sender, &recipientsStr, &ccStr, &bccStr, &messageId, &replyTo, &subject, &hdrs)
	if err != nil {
		return nil, err
	}

	msg := new(imap.MessageInfo)
	msg.UID = uid
	msg.Msg.Date = time.Unix(timestamp, 0)
	msg.Msg.Subject = subject
	senderAddr, err := mail.ParseAddress(sender)
	if err == nil {
		msg.Msg.From = common.Address(*senderAddr)
	}
	toList, err := mail.ParseAddressList(recipientsStr)
	if err == nil {
		msg.Msg.To = convAddrList(toList)
	}
	ccList, err := mail.ParseAddressList(ccStr)
	if err == nil {
		msg.Msg.Cc = convAddrList(ccList)
	}
	bccList, err := mail.ParseAddressList(bccStr)
	if err == nil {
		msg.Msg.Bcc = convAddrList(bccList)
	}
	replyAddr, err := mail.ParseAddress(replyTo)
	if err == nil {
		msg.Msg.ReplyTo = common.Address(*replyAddr)
	}

	msg.Msg.Misc, err = common.ReadHeader(hdrs)

	return msg, nil
}

func readTagList(out *imap.MessageInfo, in *sql.Rows) error {
	out.CustomTags = []string{}
	for in.Next() {
		tag := ""
		err := in.Scan(&tag)
		if err != nil {
			return err
		}

		switch tag {
		case eimap.SeenFlag:
			out.Readen = true
		case eimap.AnsweredFlag:
			out.Answered = true
		case eimap.RecentFlag:
			out.Recent = true
		default:
			out.CustomTags = append(out.CustomTags, tag)
		}
	}
	return nil
}

func readPartInfo(out *imap.MessageInfo, in *sql.Rows) error {
	out.Msg.Parts = []common.Part{}
	for in.Next() {
		attachment, contentType, contentSubtype, contentTypeParams, size, filename, hdrs := 0, "", "", "", 0, "", []byte{}
		in.Scan(&attachment, &contentType, &contentSubtype, &size, &filename, hdrs)

		part := common.Part{}
		part.Type, _ = common.ParseParamHdr(contentType + "/" + contentSubtype + ";" + contentTypeParams)
		part.Size = uint32(size)

		hdrsParsed, err := common.ReadHeader(hdrs)
		if err != nil {
			if err == io.EOF {
				hdrsParsed = make(common.Header)
			} else {
				return err
			}
		}
		v, params, _ := hdrsParsed.ContentDisposition()
		part.Disposition = common.ParametrizedHeader{v, params}
		hdrsParsed.Del("Content-Disposition")
		part.Misc = hdrsParsed

		out.Msg.Parts = append(out.Msg.Parts, part)
	}
	return nil
}

// InvalidateMsglist resets UIDVALIDITY, unreadcounts and remove all messages from cache.
func (d *Dirwrapper) InvalidateMsglist() error {
	tx, err := d.parent.d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Stmt(d.parent.invalidateMsglist).Exec(d.dir); err != nil {
		return err
	}
	if _, err := tx.Stmt(d.parent.remDirMsgs).Exec(d.dir, d.dir); err != nil {
		return err
	}
	if _, err := tx.Stmt(d.parent.remDirMsgs2).Exec(d.dir); err != nil {
		return err
	}
	if _, err := tx.Stmt(d.parent.remDirMsgs3).Exec(d.dir); err != nil {
		return err
	}

	return tx.Commit()
}

func (d *Dirwrapper) GetMsg(uid uint32) (*imap.MessageInfo, error) {
	row := d.parent.getMsgByUid.QueryRow(d.dir, uid)

	msg, err := readMessageInfo(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNullValue
		}
		return nil, err
	}

	rows, err := d.parent.getMsgTags.Query(d.dir, uid)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil {
		if err := readTagList(msg, rows); err != nil {
			return nil, err
		}
	} else {
		msg.CustomTags = []string{}
	}
	rows, err = d.parent.getMsgPartInfo.Query(d.dir, uid)
	if err != nil {
		return nil, err
	}
	if err := readPartInfo(msg, rows); err != nil {
		return nil, err
	}
	return msg, nil
}

func (d *Dirwrapper) DelMsg(uid uint32) error {
	tx, err := d.parent.d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = d.parent.delMsgParts.Exec(d.dir, uid)
	if err != nil {
		return err
	}

	_, err = d.parent.delMsgTags.Exec(d.dir, uid)
	if err != nil {
		return err
	}

	_, err = d.parent.delMsg.Exec(d.dir, uid)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (d *Dirwrapper) AddTag(uid uint32, tag string) error {
	_, err := d.parent.addTag.Exec(d.dir, uid, tag)
	return err
}

func (d *Dirwrapper) RemTag(uid uint32, tag string) error {
	_, err := d.parent.remTag.Exec(d.dir, uid, tag)
	return err
}

func (d *Dirwrapper) GetPartBody(uid uint32, indx uint) ([]byte, error) {
	row := d.parent.getMsgPart.QueryRow(d.dir, uid, indx)
	out := []byte{}
	return out, row.Scan(out)
}

func (d *Dirwrapper) AddMsg(msg *imap.MessageInfo) error {
	tx, err := d.parent.d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := d.addMsg(tx, msg); err != nil {
		return err
	}

	return tx.Commit()
}

func (d *Dirwrapper) addMsg(tx *sql.Tx, msg *imap.MessageInfo) error {
	unixStamp := msg.Msg.Date.Unix()

	hdrs, err := common.WriteHeader(msg.Msg.Misc)
	if err != nil {
		return err
	}

	_, err = tx.Stmt(d.parent.addMsg).Exec(d.dir, msg.UID, unixStamp, common.MarshalAddress(msg.Msg.From),
		common.MarshalAddressList(msg.Msg.To), common.MarshalAddressList(msg.Msg.Cc),
		common.MarshalAddressList(msg.Msg.Bcc) /* TODO: msgid */, "", common.MarshalAddress(msg.Msg.ReplyTo),
		msg.Msg.Subject, hdrs)
	if err != nil {
		return err
	}

	if msg.Readen {
		if _, err := tx.Stmt(d.parent.addTag).Exec(d.dir, msg.UID, eimap.SeenFlag); err != nil {
			return err
		}
	}
	if msg.Answered {
		if _, err := tx.Stmt(d.parent.addTag).Exec(d.dir, msg.UID, eimap.AnsweredFlag); err != nil {
			return err
		}
	}
	if msg.Recent {
		if _, err := tx.Stmt(d.parent.addTag).Exec(d.dir, msg.UID, eimap.RecentFlag); err != nil {
			return err
		}
	}
	for _, tag := range msg.CustomTags {
		if _, err := tx.Stmt(d.parent.addTag).Exec(d.dir, msg.UID, tag); err != nil {
			return err
		}
	}

	for i, prt := range msg.Msg.Parts {
		if err := d.addPart(tx, msg.UID, uint(i), &prt); err != nil {
			return err
		}
	}

	return nil
}

func (d *Dirwrapper) ReplaceTagList(msgUid uint32, newList []string) error {
	tx, err := d.parent.d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Stmt(d.parent.delMsgTags).Exec(d.dir, msgUid); err != nil {
		return err
	}

	return tx.Commit()
}

func (d *Dirwrapper) ReplacePartList(msgUid uint32, newParts []common.Part) error {
	tx, err := d.parent.d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM parts WHERE dir = ? AND uid = ?`, d.dir, msgUid); err != nil {
		return err
	}

	for i, prt := range newParts {
		if err := d.addPart(tx, msgUid, uint(i), &prt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *Dirwrapper) MsgsCount() uint {
	row := d.parent.d.QueryRow(`SELECT COUNT() FROM meta WHERE dir = ?`, d.dir)
	count := uint(0)
	if err := row.Scan(&count); err != nil {
		// wtf
		panic(err)
	}
	return count
}

func (d *Dirwrapper) addPart(tx *sql.Tx, msgUid uint32, indx uint, prt *common.Part) error {
	hdrsBytes, err := common.WriteHeader(prt.Misc)
	if err != nil {
		return err
	}

	size := prt.Size
	if prt.Body != nil {
		size = uint32(len(prt.Body))
	}

	typeParts := strings.Split(prt.Type.Value, "/")
	if len(typeParts) == 1 {
		typeParts = []string{"", ""}
	}
	type_, subtype := typeParts[0], typeParts[1]
	typeParams := []string{}
	for name, value := range prt.Type.Params {
		typeParams = append(typeParams, fmt.Sprintf("%v=%v", name, value))
	}

	attachment := 0
	filename := ""
	if prt.Disposition.Value == "attachment" {
		attachment = 1
		if f, prs := prt.Disposition.Params["filename"]; prs {
			filename = f
		}
	}

	if tx != nil {
		_, err = tx.Stmt(d.parent.addPart).Exec(d.dir, msgUid, indx, attachment, type_, subtype, strings.Join(typeParams, "; "), size, filename, hdrsBytes, prt.Body)
	} else {
		_, err = d.parent.addPart.Exec(d.dir, msgUid, indx, attachment, type_, subtype, strings.Join(typeParams, "; "), size, filename, hdrsBytes, prt.Body)
	}
	return err
}

func (d *Dirwrapper) AddPart(msgUid uint32, indx uint, prt *common.Part) error {
	return d.addPart(nil, msgUid, indx, prt)
}

func (d *Dirwrapper) ResolveUid(seq uint32) (uint32, error) {
	row := d.parent.resolveUid.QueryRow(d.dir, seq)
	uid := uint32(0)
	return uid, row.Scan(&uid)
}
