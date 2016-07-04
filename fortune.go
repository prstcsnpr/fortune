package main

import (
	"database/sql"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/axgle/mahonia"
	_ "github.com/mattn/go-sqlite3"
	"net/http"
	"os"
	"strings"
)

func NewDocument(url string) (*goquery.Document, error) {
	response, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	decoder := mahonia.NewDecoder("gb18030")
	reader := decoder.NewReader(response.Body)
	document, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, err
	}
	return document, nil
}

func UpdateStockBasicInfoList(file string) error {
	document, err := NewDocument("http://quote.eastmoney.com/stocklist.html")
	if err != nil {
		return err
	}
	selection := document.Find("#quotesearch ul li a")
	selection.Each(func(i int, s *goquery.Selection) {
		left := strings.Replace(s.Text(), "(", " ", -1)
		right := strings.Replace(left, ")", " ", -1)
		item := strings.Fields(strings.TrimSpace(right))
		if 2 == len(item) && 6 == len(item[1]) {
			char := item[1][0]
			if char == '0' || char == '6' || char == '3' {
				err = UpdateStockTitle(file, item[1], item[0])
				if err != nil {
					fmt.Println(err)
				}
			}
		}
	})
	return nil
}

func UpdateStockTitle(file string, ticker string, title string) error {
	db, err := sql.Open("sqlite3", file)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	insertStmt, err := tx.Prepare("insert or ignore into stock_basic_info (ticker, title) values (?,?)")
	if err != nil {
		return err
	}
	defer insertStmt.Close()
	_, err = insertStmt.Exec(ticker, title)
	if err != nil {
		return err
	}
	updateStmt, err := tx.Prepare("update stock_basic_info set title=? where ticker=?")
	if err != nil {
		return err
	}
	defer updateStmt.Close()
	_, err = updateStmt.Exec(title, ticker)
	tx.Commit()
	return nil
}

func main() {
	file := os.Args[1]
	err := UpdateStockBasicInfoList(file)
	if err != nil {
		fmt.Println(err)
		return
	}
	//UpdateStockBasicInfoList(file, stockTitleList)
}
