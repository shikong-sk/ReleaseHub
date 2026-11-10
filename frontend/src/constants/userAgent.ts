// 下载 User-Agent 预设：与后端 config/useragent.go 的 builtinUserAgentPresets 保持一致。
// 用途：设置页（全局 UA）与仓库编辑抽屉（仓库级 UA 覆盖）共用同一份展示定义，
// 避免两处硬编码不一致；后端 GET /api/config 会下发权威列表，前端仅在缺失时兜底使用本表。
export interface UserAgentPresetOption {
  /** 持久化值，如 default / chrome-win / custom */
  value: string
  /** 下拉展示名 */
  label: string
  /** 实际 UA 串；custom 项为空（由用户填写） */
  ua: string
}

export const USER_AGENT_PRESETS: UserAgentPresetOption[] = [
  { value: 'default', label: '默认（ReleaseHub）', ua: 'ReleaseHub' },
  {
    value: 'chrome-win',
    label: 'Chrome / Windows',
    ua: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36'
  },
  {
    value: 'edge-win',
    label: 'Edge / Windows',
    ua: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0'
  },
  {
    value: 'firefox-win',
    label: 'Firefox / Windows',
    ua: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0'
  },
  {
    value: 'safari-mac',
    label: 'Safari / macOS',
    ua: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15'
  },
  {
    value: 'chrome-android',
    label: 'Chrome / Android',
    ua: 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36'
  },
  { value: 'custom', label: '自定义', ua: '' }
]

// USER_AGENT_FOLLOW_GLOBAL 仓库级「跟随全局」哨兵值（空串语义，仅用于下拉展示）
export const USER_AGENT_FOLLOW_GLOBAL = ''

// buildUserAgentOptions 构造下拉选项；withFollowGlobal 为 true 时在首位插入「跟随全局配置」
export function buildUserAgentOptions(
  presets: UserAgentPresetOption[] = USER_AGENT_PRESETS,
  withFollowGlobal = false
): { label: string; value: string }[] {
  const options = presets.map((item) => ({ label: item.label, value: item.value }))
  if (withFollowGlobal) {
    options.unshift({ label: '跟随全局配置', value: USER_AGENT_FOLLOW_GLOBAL })
  }
  return options
}

// resolveUserAgentPreview 根据 preset + 自定义值给出可读的 UA 预览（用于只读态展示）
export function resolveUserAgentPreview(
  preset: string,
  customUA: string,
  presets: UserAgentPresetOption[] = USER_AGENT_PRESETS
): string {
  // 无 preset 或 custom 但未填值 → 空串表示「跟随全局配置」（由调用方展示提示文案）
  if (!preset) return ''
  if (preset === 'custom') return customUA.trim()
  if (preset === 'default') return 'ReleaseHub'
  const hit = presets.find((item) => item.value === preset)
  return hit?.ua || 'ReleaseHub'
}
