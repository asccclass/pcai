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

func ExportFormatGoogleEventForLLM(e googleCalendarEventRaw) string {
	return formatGoogleEventForLLM(e)
}

func ExportProcessGoogleAPIEvents(rawEvents []json.RawMessage) string {
	return processGoogleAPIEvents(rawEvents)
}

func ExportFormatCalendarEvent(creator string, e calendarEvent) string {
	return formatCalendarEvent(creator, e)
}

// ExportCalendarEventRaw 導出 googleCalendarEventRaw 結構 (Google API 格式)
type ExportCalendarEventRaw = googleCalendarEventRaw

// ExportCalendarEvent 導出 calendarEvent 結構 (calendar.exe 格式)
type ExportCalendarEvent = calendarEvent
