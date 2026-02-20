package skillloader

import "encoding/json"

// 導出未導出的函式供外部測試使用。
// 僅供系統測試使用，不應在正式程式碼中呼叫。

func ExportPostProcessCalendarOutput(output string) string {
	return postProcessCalendarOutput(output)
}

func ExportFixExclusiveEndDate(startDate, endDate string) string {
	return fixExclusiveEndDate(startDate, endDate)
}

func ExportFormatEventForLLM(e calendarEventRaw) string {
	return formatEventForLLM(e)
}

func ExportProcessJSONEvents(rawEvents []json.RawMessage) string {
	return processJSONEvents(rawEvents)
}

// ExportCalendarEventRaw 導出 calendarEventRaw 結構
type ExportCalendarEventRaw = calendarEventRaw
