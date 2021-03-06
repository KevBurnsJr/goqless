package goqless

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"strconv"
)

type TaggedReply struct {
	Total int
	Jobs  StringSlice
}

type Client struct {
	conn redis.Conn
	host string
	port string

	events *Events
	lua    *Lua
}

func NewClient(host, port string) *Client {
	return &Client{host: host, port: port}
}

func Dial(host, port string) (*Client, error) {
	c := NewClient(host, port)

	conn, err := redis.Dial("tcp", fmt.Sprintf("%s:%s", host, port))
	if err != nil {
		return nil, err
	}

	c.lua = NewLua(conn)
	err = c.lua.LoadScripts("qless-core") // make get from lib path
	if err != nil {
		println(err.Error())
		conn.Close()
		return nil, err
	}

	c.conn = conn
	return c, nil
}

func (c *Client) Close() {
	c.conn.Close()
}

func (c *Client) Events() *Events {
	if c.events != nil {
		return c.events
	}
	c.events = NewEvents(c.host, c.port)
	return c.events
}

func (c *Client) Do(name string, keysAndArgs ...interface{}) (interface{}, error) {
	return c.lua.Do(name, keysAndArgs...)
}

func (c *Client) Queue(name string) *Queue {
	q := NewQueue(c)
	q.Name = name
	return q
}

func (c *Client) Queues(name string) ([]*Queue, error) {
	args := []interface{}{0, "queues", timestamp()}
	if name != "" {
		args = append(args, name)
	}

	byts, err := redis.Bytes(c.Do("qless", args...))
	if err != nil {
		return nil, err
	}

	qr := []*Queue{NewQueue(c)}
	if name == "" {
		err = json.Unmarshal(byts, &qr)
		for _, q := range qr {
			q.cli = c
		}
	} else {
		err = json.Unmarshal(byts, &qr[0])
	}

	if err != nil {
		return nil, err
	}

	return qr, err
}

// Track the jid
func (c *Client) Track(jid string) (bool, error) {
	return Bool(c.Do("qless", 0, "track", timestamp(), "track", jid, ""))
}

// Untrack the jid
func (c *Client) Untrack(jid string) (bool, error) {
	return Bool(c.Do("qless", 0, "track", timestamp(), 0, "untrack", jid))
}

// Returns all the tracked jobs
func (c *Client) Tracked() (string, error) {
	return redis.String(c.Do("qless", 0, "track", timestamp()))
}

func (c *Client) Get(jid string) (interface{}, error) {
	job, err := c.GetJob(jid)
	if err == redis.ErrNil {
		rjob, err := c.GetRecurringJob(jid)
		return rjob, err
	}
	return job, err
}

func (c *Client) GetJob(jid string) (*Job, error) {
	byts, err := redis.Bytes(c.Do("qless", 0, "get", timestamp(), jid))
	if err != nil {
		return nil, err
	}

	job := NewJob(c)
	err = json.Unmarshal(byts, job)
	if err != nil {
		return nil, err
	}
	return job, err
}

func (c *Client) GetRecurringJob(jid string) (*RecurringJob, error) {
	byts, err := redis.Bytes(c.Do("qless", 0, "recur", timestamp(), "get", jid))
	if err != nil {
		return nil, err
	}

	job := NewRecurringJob(c)
	err = json.Unmarshal(byts, job)
	if err != nil {
		return nil, err
	}
	return job, err
}

func (c *Client) Completed(start, count int) ([]string, error) {
	reply, err := redis.Values(c.Do("qless", 0, "jobs", timestamp(), "complete"))
	if err != nil {
		return nil, err
	}

	ret := []string{}
	for _, val := range reply {
		s, _ := redis.String(val, err)
		ret = append(ret, s)
	}
	return ret, err
}

func (c *Client) Tagged(tag string, start, count int) (*TaggedReply, error) {
	byts, err := redis.Bytes(c.Do("qless", 0, "tag", timestamp(), "get", tag, start, count))
	if err != nil {
		return nil, err
	}

	t := &TaggedReply{}
	err = json.Unmarshal(byts, t)
	return t, err
}

func (c *Client) GetConfig(option string) (string, error) {
	interf, err := c.Do("qless", 0, "config.get", timestamp(), option)
	if err != nil {
		return "", err
	}

	var contentStr string
	switch interf.(type) {
	case []uint8:
		contentStr, err = redis.String(interf, nil)
	case int64:
		var contentInt64 int64
		contentInt64, err = redis.Int64(interf, nil)
		if err == nil {
			contentStr = strconv.Itoa(int(contentInt64))
		}
	default:
		err = errors.New("The redis return type is not []uint8 or int64")
	}
	if err != nil {
		return "", err
	}

	return contentStr, err
}

func (c *Client) SetConfig(option string, value interface{}) {
	intf, err := c.Do("qless", 0, "config.set", timestamp(), option, value)
	if err != nil {
		fmt.Println("setconfig, c.Do fail. interface:", intf, " err:", err)
	}
}

func (c *Client) UnsetConfig(option string) {
	c.Do("qless", 0, "config.unset", timestamp(), option)
}
