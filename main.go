package main

import (
	"encoding/csv"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
)

func readCsvFile(filePath string) [][]string {
	// https://stackoverflow.com/a/58841827
	f, err := os.Open(filePath)
	if err != nil {
		log.Fatal("Unable to read input file "+filePath, err)
	}
	defer f.Close()

	csvReader := csv.NewReader(f)
	records, err := csvReader.ReadAll()
	if err != nil {
		log.Fatal("Unable to parse file as CSV for "+filePath, err)
	}

	return records
}

func generateLineItems(vals []float64) []opts.LineData {
	items := make([]opts.LineData, 0)
	for i := 0; i < len(vals); i++ {
		items = append(items, opts.LineData{Value: vals[i]})
	}
	return items
}

func httpserver(w http.ResponseWriter, _ *http.Request) {
	dates := []string{}
	dat := map[string][]float64{} //{"hello": "world", …}
	srcdat := readCsvFile("binance_apr.csv")
	for _, line := range srcdat[1:] {
		// fmt.Println(line[0], line[1], line[2])
		v, _ := strconv.ParseFloat(line[2], 64)
		dat[line[1]] = append(dat[line[1]], v)
		if len(dates) == 0 || dates[len(dates)-1] != line[0] {
			dates = append(dates, line[0])
		}
	}
	// https://stackoverflow.com/a/62079701
	// bs, _ := json.Marshal(dat)
	// fmt.Println(string(bs))

	// create a new line instance
	line := charts.NewLine()
	// set some global options like Title/Legend/ToolTip or anything else
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Binance Earn APR History",
			Subtitle: "Line chart rendered by the http server this time",
		}),
		charts.WithLegendOpts(opts.Legend{Show: true}),
	)

	// Put data into instance
	line.SetXAxis(dates).SetSeriesOptions(
		charts.WithLineChartOpts(opts.LineChart{Smooth: true}),
		// charts.WithLabelOpts(opts.Label{Show: true}),
	)
	for asset, vals := range dat {
		line.AddSeries(asset, generateLineItems(vals))
	}
	line.Render(w)
}

func main() {
	http.HandleFunc("/", httpserver)
	// Open port in firewall https://linuxconfig.org/how-to-allow-port-through-firewall-on-almalinux
	// firewall-cmd --zone=public --add-port 8081/tcp --permanent
	// firewall-cmd --reload
	// firewall-cmd --zone=public --list-ports
	http.ListenAndServe(":8081", nil)
}
