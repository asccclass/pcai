---
name: get_taiwan_weather
description: 從 Google Sheets 資料庫中查詢台灣各縣市指定地區的目前及未來天氣預報。
command: fetch_url "https://script.google.com/macros/s/AKfycbyR1nCx7yYQHgXOlZ5ko_ucbSeyJhDIp-PYxQ8rPDSdexz0I1LrDotZbvpBLZp6YpizYw/exec?location={{url:location}}"
cache_duration: 3h
options:
  location:
    - 基隆市
    - 臺北市
    - 新北市
    - 桃園市
    - 新竹市
    - 新竹縣
    - 苗栗縣
    - 臺中市
    - 彰化縣
    - 南投縣
    - 雲林縣
    - 嘉義市
    - 嘉義縣
    - 臺南市
    - 高雄市
    - 屏東縣
    - 宜蘭縣
    - 花蓮縣
    - 臺東縣
    - 澎湖縣
    - 金門縣
    - 連江縣
option_aliases:
  location:
    "中正區": "臺北市"
    "大同區": "臺北市"
    "中山區": "臺北市"
    "松山區": "臺北市"
    "大安區": "臺北市"
    "萬華區": "臺北市"
    "信義區": "臺北市"
    "士林區": "臺北市"
    "北投區": "臺北市"
    "內湖區": "臺北市"
    "南港區": "臺北市"
    "文山區": "臺北市"
    "苗栗市": "苗栗縣"
    "彰化市": "彰化縣"
    "南投市": "南投縣"
    "雲林市": "雲林縣"
    "嘉義市": "嘉義縣"
    "屏東市": "屏東縣"
    "台東市": "台東縣"
    "花蓮市": "花蓮縣"
    "澎湖市": "澎湖縣"
    "金門市": "金門縣"
    "苗栗": "苗栗縣"
    "彰化": "彰化縣"
    "南投": "南投縣"
    "雲林": "雲林縣"
    "嘉義": "嘉義縣"
    "屏東": "屏東縣"
    "宜蘭": "宜蘭縣"
    "花蓮": "花蓮縣"
    "台東": "臺東縣"
    "澎湖": "澎湖縣"
    "金門": "金門縣"
    "連江": "連江縣"
    "基隆": "基隆市"
    "新竹": "新竹市"
    "台中": "臺中市"
    "高雄": "高雄市"
    "台北": "臺北市"
    "新北": "新北市"
    "台南": "臺南市"
    "桃園": "桃園市"
    "台北市": "臺北市"
    "台中市": "臺中市"
    "台南市": "臺南市"
---

# 查詢台灣地區天氣預報 (get_taiwan_weather)

從 Google Sheets 資料庫中查詢台灣各縣市指定地區的**目前時段及未來天氣預報**。系統會自動過濾掉已過期的時段。

## 參數描述
- `location`: (string, required) 台灣的地區名稱。必須是合法行政區名稱 (如：臺北市、高雄市)。系統會自動校正些微的輸入錯誤。

## 呼叫範例
fetch_url "https://script.google.com/macros/s/AKfycbyR1nCx7yYQHgXOlZ5ko_ucbSeyJhDIp-PYxQ8rPDSdexz0I1LrDotZbvpBLZp6YpizYw/exec?location=臺北市"

## 回傳格式說明
回傳為 JSON 格式，包含該地區**目前時段及未來**的預報資訊：
- `city`: 地區名稱
- `queryTime`: 查詢當下的時間
- `forecast`: 未來預報列表
    - `StartTime`: 預報開始時間
    - `EndTime`: 預報結束時間
    - `Weather Description`: 詳細天氣描述
    - `ProbabilityOfPrecipitation`: 降雨機率