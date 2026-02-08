---
name: get_taiwan_weather
description: 從 Google Sheets 資料庫中查詢台灣各縣市指定地區的未來天氣預報。
command: fetch_url "https://script.google.com/macros/s/AKfycbyR1nCx7yYQHgXOlZ5ko_ucbSeyJhDIp-PYxQ8rPDSdexz0I1LrDotZbvpBLZp6YpizYw/exec?location={{location}}"
---

# 查詢台灣地區天氣預報 (get_taiwan_weather)

從 Google Sheets 資料庫中查詢台灣各縣市指定地區的**未來天氣預報**。系統會自動過濾掉已過期的時段。

## 參數描述
- `location`: (string, required) 台灣的地區名稱，必須與 Google Sheet 的分頁名稱一致。例如：「臺北市」、「基隆市」、「高雄縣」。

## 呼叫http_get工具
fetch_url "https://script.google.com/macros/s/AKfycbyR1nCx7yYQHgXOlZ5ko_ucbSeyJhDIp-PYxQ8rPDSdexz0I1LrDotZbvpBLZp6YpizYw/exec?location=臺北市"

## 回傳格式說明
回傳為 JSON 格式，包含該地區**目前時間點之後**的前三筆預報資訊：
- `city`: 地區名稱
- `queryTime`: 查詢當下的時間
- `forecast`: 未來預報列表
    - `StartTime`: 預報開始時間
    - `EndTime`: 預報結束時間
    - `Weather Description`: 詳細天氣描述
    - `ProbabilityOfPrecipitation`: 降雨機率