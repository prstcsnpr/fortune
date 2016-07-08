package main

import (
	"database/sql"
	"flag"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/axgle/mahonia"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
)

func NewBody(url string) (string, error) {
	response, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	decoder := mahonia.NewDecoder("gb18030")
	reader := decoder.NewReader(response.Body)
	body, err := ioutil.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

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

func UpdateStockBasicInfoList(file string, updateAll bool) error {
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
			if strings.HasPrefix(item[1], "000") || strings.HasPrefix(item[1], "002") || char == '6' || char == '3' {
				fmt.Printf("%d %s %s\n", i, item[1], item[0])
				err = UpdateStockTitle(file, item[1], item[0], updateAll)
				if err != nil {
					fmt.Println(err)
				}
			}
		}
	})
	return nil
}

func ParseStockEarningsBody(url string) (map[string]map[string]string, error) {
	body, err := NewBody(url)
	if err != nil {
		return nil, err
	}
	result := make(map[int]map[string]string)
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		for i := 0; i < len(fields)-2; i++ {
			if _, ok := result[i+1]; !ok {
				result[i+1] = make(map[string]string)
			}
			result[i+1][fields[0]] = fields[i+1]
		}
	}
	results := make(map[string]map[string]string)
	for k := range result {
		if _, ok := result[k]["报表日期"]; ok {
			results[result[k]["报表日期"]] = result[k]
		}
	}
	return results, nil
}

func ObtainStockBalanceEarnings(ticker string) (map[string]map[string]string, error) {
	return ParseStockEarningsBody("http://money.finance.sina.com.cn/corp/go.php/vDOWN_BalanceSheet/displaytype/4/stockid/" + ticker + "/ctrl/all.phtml")
}

func ObtainStockProfitEarnings(ticker string) (map[string]map[string]string, error) {
	return ParseStockEarningsBody("http://money.finance.sina.com.cn/corp/go.php/vDOWN_ProfitStatement/displaytype/4/stockid/" + ticker + "/ctrl/all.phtml")
}

func ObtainStockMarketCapital(ticker string) (float64, error) {
	stock := ""
	if ticker[0] == '6' {
		stock = "sh" + ticker
	} else if ticker[0] == '3' || ticker[0] == '0' {
		stock = "sz" + ticker
	}
	body, err := NewBody("http://qt.gtimg.cn/S?q=" + stock)
	if err != nil {
		return 0, err
	}
	fields := strings.Split(body, "~")
	value := 0.0
	if len(fields)-5 < 0 || fields[len(fields)-5] == "" {
		value = -1
	} else {
		value, err = strconv.ParseFloat(fields[len(fields)-6], 64)
		if err != nil {
			return 0, err
		}
		value *= 100000000
	}
	return value, nil
}

func Query(file string, query string, f func(*sql.Rows) error, args ...interface{}) error {
	db, err := sql.Open("sqlite3", file)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	err = f(rows)
	if err != nil {
		return err
	}
	return nil
}

func GetStockEarning(file string, ticker string) (map[string]map[string]float64, error) {
	results := make(map[string]map[string]float64)
	f := func(rows *sql.Rows) error {
		for rows.Next() {
			var date string
			var np float64
			var equities float64
			fmt.Println("fuck")
			err := rows.Scan(&date, &np, &equities)
			fmt.Printf("%s %f %f\n", date, np, equities)
			if err != nil {
				fmt.Println(err)
				return err
			}
			results[date]["np_parent_company_owners"] = np
			results[date]["equities_parent_company_owners"] = equities
		}
		return nil
	}
	err := Query(file, "select date, np_parent_company_owners, equities_parent_company_owners from stock_earnings where ticker = ?", f, ticker)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func UpdateStockFieldEarnings(file string, ticker string, date string, value float64, field string) error {
	err := Exec(file, "insert or ignore into stock_earnings (ticker, date, "+field+") values (?,?,?)", ticker, date, value)
	if err != nil {
		return err
	}
	err = Exec(file, "update stock_earnings set "+field+"=? where ticker=? and date=?", value, ticker, date)
	if err != nil {
		return err
	}
	return nil
}

func UpdateStockBalanceEarnings(file string, ticker string) error {
	balance, err := ObtainStockBalanceEarnings(ticker)
	if err != nil {
		return err
	}
	var equities_parent_company_owners float64
	for k := range balance {
		if _, ok := balance[k]["归属于母公司股东权益合计"]; ok {
			equities_parent_company_owners, err = strconv.ParseFloat(balance[k]["归属于母公司股东权益合计"], 64)
			if err != nil {
				return err
			}
		} else {
			equities_parent_company_owners, err = strconv.ParseFloat(balance[k]["归属于母公司股东的权益"], 64)
			if err != nil {
				return err
			}
		}
		err = UpdateStockFieldEarnings(file, ticker, k, equities_parent_company_owners, "equities_parent_company_owners")
		if err != nil {
			return err
		}
	}
	return nil
}

func UpdateStockProfitEarnings(file string, ticker string) error {
	profit, err := ObtainStockProfitEarnings(ticker)
	if err != nil {
		return err
	}
	var np_parent_company_owners float64
	for k := range profit {
		if _, ok := profit[k]["归属于母公司所有者的净利润"]; ok {
			np_parent_company_owners, err = strconv.ParseFloat(profit[k]["归属于母公司所有者的净利润"], 64)
			if err != nil {
				return err
			}
		} else {
			np_parent_company_owners, err = strconv.ParseFloat(profit[k]["归属于母公司的净利润"], 64)
			if err != nil {
				return err
			}
		}
		err = UpdateStockFieldEarnings(file, ticker, k, np_parent_company_owners, "np_parent_company_owners")
		if err != nil {
			return err
		}
	}
	return nil
}

func UpdateStockEarnings(file string, ticker string) error {
	err := UpdateStockProfitEarnings(file, ticker)
	if err != nil {
		return err
	}
	err = UpdateStockBalanceEarnings(file, ticker)
	if err != nil {
		return err
	}
	return nil
}

func UpdateStockMarketCapital(file string, ticker string, value float64, updateAll bool) error {
	err := Exec(file, "update stock_basic_info set market_capital=? where ticker=?", value, ticker)
	if err != nil {
		return err
	}
	if updateAll {
		err = UpdateStockEarnings(file, ticker)
		if err != nil {
			return err
		}
	}
	return nil
}

func Exec(file string, query string, args ...interface{}) error {
	db, err := sql.Open("sqlite3", file)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(query, args...)
	if err != nil {
		return err
	}
	return nil
}

func UpdateStockTitle(file string, ticker string, title string, updateAll bool) error {
	err := Exec(file, "insert or ignore into stock_basic_info (ticker, title) values (?,?)", ticker, title)
	if err != nil {
		return err
	}
	err = Exec(file, "update stock_basic_info set title=? where ticker=?", title, ticker)
	if err != nil {
		return err
	}
	value, err := ObtainStockMarketCapital(ticker)
	if err != nil {
		return err
	}
	err = UpdateStockMarketCapital(file, ticker, value, updateAll)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	db := flag.String("d", "fortune.db", "use -d <db data path>")
	updateAll := flag.Bool("ua", false, "use -ua")
	show := flag.Bool("s", false, "use -s")
	flag.Parse()
	if *show {
		_, err := GetStockEarning(*db, "600216")
		if err != nil {
			fmt.Println(err)
		}
	} else {
		err := UpdateStockBasicInfoList(*db, *updateAll)
		if err != nil {
			fmt.Println(err)
			return
		}
	}
}
