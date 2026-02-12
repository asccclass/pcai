---
name: task_planning
description: 用於分析複雜任務並將其分解為可執行的步驟計畫。
helper: true
---

# 任務規劃技能 (Task Planning Skill)

當使用者提出一個 **複雜、多步驟** 或 **需要長時間執行** 的請求時 (例如 "分析這個專案並整合功能"、"研究 X 主題並寫報告")，你應該使用此技能來進行規劃與追蹤。

## 使用時機

- 使用者請求無法透過單一次回答完成。
- 需要多個工具配合 (例如：先搜尋 -> 讀取 -> 分析 -> 寫入)。
- 需要確保任務不會因為 Context Window 限制而遺忘進度。

## 操作流程

1.  **建立計畫 (Create)**
    - 分析使用者的目標。
    - 將目標拆解為 3-10 個具體的步驟。
    - 呼叫 `task_planner` 工具，action="create"。
    - 範例: `task_planner(action="create", goal="研究 OpenClaw", steps="搜尋 Github;閱讀 README;分析架構;總結報告")`

2.  **執行與更新 (Execute & Update)**
    - 執行每一個步驟 (例如呼叫 `search_web` 或 `view_file`)。
    - 每完成一個步驟，呼叫 `task_planner` 更新狀態。
    - 範例: `task_planner(action="update", step_id=1, status="completed", result="已找到相關文件...")`

3.  **檢查進度 (Get)**
    - 如果你忘記當前進度 (例如在多輪對話後)，呼叫 `task_planner(action="get")` 來回憶。

4.  **完成任務 (Finish)**
    - 當所有步驟都完成，且你已經向使用者回報最終結果後。
    - 呼叫 `task_planner(action="finish")` 清除計畫。

## 這裡有些原則

- **不要過度規劃**: 簡單的問題直接回答就好。
- **保持靈活**: 如果執行中發現計畫不對，可以使用 `update` 標記某步驟為 `skipped` 或 `failed`，甚至重新 create 一個新計畫。
- **自我檢視**: 每次執行工具前，想一下這是否符合當前計畫的下一步。
