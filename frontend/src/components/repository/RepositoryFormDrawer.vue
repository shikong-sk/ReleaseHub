<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from 'vue'
import {
  NAlert,
  NButton,
  NDrawer,
  NDrawerContent,
  NForm,
  NFormItem,
  NButtonGroup,
  NInput,
  NInputNumber,
  NModal,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  useMessage
} from 'naive-ui'
import type { SelectOption } from 'naive-ui'

import { previewFilter, type FilterPreviewResult } from '@/api/filter'
import { useTokensStore } from '@/stores/tokens'
import { useStoragesStore } from '@/stores/storages'
import { useProxiesStore } from '@/stores/proxies'
import type { Repository, RepositoryFilterMode, RepositoryFormMode, RepositoryPayload } from '@/types/repository'

const props = defineProps<{
  show: boolean
  mode: RepositoryFormMode
  repository: Repository | null
  saving: boolean
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  submit: [payload: RepositoryPayload]
}>()

const message = useMessage()
const showPreview = shallowRef(false)
const previewLoading = shallowRef(false)
const previewResults = shallowRef<FilterPreviewResult[]>([])
const previewError = shallowRef<string | null>(null)
const selectedFilterPresets = shallowRef<string[]>([])

const tokensStore = useTokensStore()
const storagesStore = useStoragesStore()
const proxiesStore = useProxiesStore()

const selectedTokenId = computed<number>({
  get: () => form.githubTokenId ?? 0,
  set: (val: number) => {
    form.githubTokenId = val === 0 ? null : val
  }
})

const selectedStorageIds = computed<number[]>({
  get: () => {
    // 优先使用 storageIds，回退到 storageId
    const ids = form.storageIds ?? []
    if (ids.length > 0) return ids
    if (form.storageId != null) return [form.storageId]
    return []
  },
  set: (val: number[]) => {
    form.storageIds = val
    form.storageId = val.length > 0 ? val[0] : null
  }
})

const selectedProxyId = computed<number>({
  get: () => form.proxyId ?? 0,
  set: (val: number) => {
    form.proxyId = val === 0 ? null : val
  }
})

const form = reactive<RepositoryPayload>({
  provider: 'github',
  owner: '',
  repo: '',
  githubTokenId: null,
  storageId: null,
  storageIds: [] as number[],
  proxyId: null,
  providerApiBaseUrl: '',
  enabled: true,
  intervalSeconds: 1800,
  filterMode: 'glob',
  assetIncludePatterns: '',
  assetExcludePatterns: '',
  tagFilterMode: '',
  tagIncludePattern: '',
  tagExcludePattern: '',
  retentionKeepLatest: 5
})

const title = computed(() => (props.mode === 'create' ? '新增 GitHub 仓库' : '编辑仓库'))
const ownerDisabled = computed(() => props.mode === 'edit')

const providerOptions = computed<SelectOption[]>(() => [
    { label: 'GitHub', value: 'github' },
    { label: 'GitLab', value: 'gitlab' },
    { label: 'Gitea', value: 'gitea' },
    { label: 'Forgejo', value: 'forgejo' }
  ])

const tokenOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = [
    { label: '无 Token（使用匿名请求）', value: 0 }
  ]
  for (const token of tokensStore.items) {
    options.push({
      label: `${token.name} (${token.tokenHint})`,
      value: token.id
    })
  }
  return options
})

const filterPresetOptions = computed<SelectOption[]>(() => [
  { label: 'Linux AMD64 压缩包', value: 'linux-amd64-archive' },
  { label: 'Linux ARM64 压缩包', value: 'linux-arm64-archive' },
  { label: 'Windows AMD64', value: 'windows-amd64' },
  { label: 'macOS ARM64', value: 'darwin-arm64' },
  { label: '校验文件', value: 'checksums' },
  { label: '排除调试/源码/签名', value: 'exclude-debug-source' }
])

const intervalPresets = [
  { label: '5分钟', value: 300 },
  { label: '15分钟', value: 900 },
  { label: '30分钟', value: 1800 },
  { label: '1小时', value: 3600 },
  { label: '2小时', value: 7200 },
  { label: '4小时', value: 14400 },
  { label: '8小时', value: 28800 },
  { label: '12小时', value: 43200 }
]

const tagFilterPresetOptions = computed<SelectOption[]>(() => [
  { label: '只同步 v* 版本', value: 'v-only' },
  { label: '排除预发布 alpha/beta/rc', value: 'exclude-pre' },
  { label: '只同步正式版本（无后缀）', value: 'stable-only' }
])

function applyTagFilterPreset(values: string[]) {
  for (const value of values) {
    if (value === 'v-only') {
      form.tagFilterMode = 'glob'
      form.tagIncludePattern = mergePatternLines(form.tagIncludePattern ?? '', 'v*')
    } else if (value === 'exclude-pre') {
      form.tagFilterMode = form.tagFilterMode === '' ? 'glob' : form.tagFilterMode
      form.tagExcludePattern = mergePatternLines(form.tagExcludePattern ?? '', '*-alpha*\n*-beta*\n*-rc*\n*-pre*')
    } else if (value === 'stable-only') {
      form.tagFilterMode = 'regex'
      form.tagIncludePattern = mergePatternLines(form.tagIncludePattern ?? '', '^v?\\d+\\.\\d+\\.\\d+$')
    }
  }
}

const storageOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = []
  for (const storage of storagesStore.items) {
    options.push({
      label: `${storage.name} (${storage.type.toUpperCase()})`,
      value: storage.id
    })
  }
  return options
})

const proxyOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = [
    { label: '不使用代理', value: 0 }
  ]
  for (const proxy of proxiesStore.items) {
    options.push({
      label: `${proxy.name} (${proxy.type.toUpperCase()} ${proxy.host}:${proxy.port})`,
      value: proxy.id
    })
  }
  return options
})

watch(
  () => [props.show, props.repository, props.mode] as const,
  () => {
    if (!props.show) return
    void Promise.all([tokensStore.refresh(), storagesStore.refresh(), proxiesStore.refresh()])
    resetForm()
  },
  { immediate: true }
)

function resetForm() {
  form.provider = props.repository?.provider ?? 'github'
  form.owner = props.repository?.owner ?? ''
  form.repo = props.repository?.repo ?? ''
  form.githubTokenId = props.repository?.githubTokenId ?? null
  form.storageId = props.repository?.storageId ?? null
  form.storageIds = props.repository?.storageIds ?? []
  form.proxyId = props.repository?.proxyId ?? null
  form.providerApiBaseUrl = props.repository?.providerApiBaseUrl ?? ''
  form.enabled = props.repository?.enabled ?? true
  form.intervalSeconds = props.repository?.intervalSeconds ?? 1800
  form.filterMode = (props.repository?.filterMode ?? 'glob') as RepositoryFilterMode
  form.assetIncludePatterns = props.repository?.assetIncludePatterns ?? ''
  form.assetExcludePatterns = props.repository?.assetExcludePatterns ?? ''
  form.tagFilterMode = props.repository?.tagFilterMode ?? ''
  form.tagIncludePattern = props.repository?.tagIncludePattern ?? ''
  form.tagExcludePattern = props.repository?.tagExcludePattern ?? ''
  form.retentionKeepLatest = props.repository?.retentionKeepLatest ?? 5
  selectedFilterPresets.value = []
}

function submit() {
  const tokenId = form.githubTokenId
  const storageId = form.storageId
  const proxyId = form.proxyId
  emit('submit', {
    provider: form.provider,
    owner: form.owner.trim(),
    repo: form.repo.trim(),
    githubTokenId: tokenId === 0 || tokenId === null ? null : tokenId,
    storageId: storageId === 0 || storageId === null ? null : storageId,
    storageIds: (form.storageIds ?? []).length > 0 ? form.storageIds : undefined,
    proxyId: proxyId === 0 || proxyId === null ? null : proxyId,
    providerApiBaseUrl: (form.providerApiBaseUrl ?? '').trim() || undefined,
    enabled: form.enabled,
    intervalSeconds: form.intervalSeconds,
    filterMode: form.filterMode,
    assetIncludePatterns: form.assetIncludePatterns.trim(),
    assetExcludePatterns: form.assetExcludePatterns.trim(),
    tagFilterMode: form.tagFilterMode || undefined,
    tagIncludePattern: (form.tagIncludePattern ?? '').trim() || undefined,
    tagExcludePattern: (form.tagExcludePattern ?? '').trim() || undefined,
    retentionKeepLatest: form.retentionKeepLatest
  })
}

function applyFilterPresets(values: string[]) {
  const presets: Record<string, { mode: RepositoryFilterMode; include?: string; exclude?: string }> = {
    'linux-amd64-archive': {
      mode: 'glob',
      include: '*linux*amd64*.tar.gz\n*linux*amd64*.zip\n*linux*x86_64*.tar.gz\n*linux*x86_64*.zip'
    },
    'linux-arm64-archive': {
      mode: 'glob',
      include: '*linux*arm64*.tar.gz\n*linux*arm64*.zip\n*linux*aarch64*.tar.gz\n*linux*aarch64*.zip'
    },
    'windows-amd64': {
      mode: 'glob',
      include: '*windows*amd64*.zip\n*windows*x86_64*.zip\n*win*amd64*.zip'
    },
    'darwin-arm64': {
      mode: 'glob',
      include: '*darwin*arm64*.zip\n*macos*arm64*.zip'
    },
    checksums: {
      mode: 'glob',
      include: 'SHA256SUMS\nchecksums.txt\n*checksum*'
    },
    'exclude-debug-source': {
      mode: 'glob',
      exclude: '*debug*\n*source*\n*.sig\n*.asc'
    }
  }

  const selectedPresets = values.map((value) => presets[value]).filter(Boolean)
  if (selectedPresets.length === 0) return

  form.filterMode = selectedPresets[0].mode
  for (const preset of selectedPresets) {
    if (preset.include !== undefined) {
      form.assetIncludePatterns = mergePatternLines(form.assetIncludePatterns, preset.include)
    }
    if (preset.exclude !== undefined) {
      form.assetExcludePatterns = mergePatternLines(form.assetExcludePatterns, preset.exclude)
    }
  }
}

function mergePatternLines(current: string, next: string) {
  const seen = new Set<string>()
  const lines: string[] = []
  for (const line of `${current}\n${next}`.split('\n')) {
    const normalized = line.trim()
    if (!normalized || seen.has(normalized)) {
      continue
    }
    seen.add(normalized)
    lines.push(normalized)
  }
  return lines.join('\n')
}

async function handlePreview() {
  previewLoading.value = true
  previewError.value = null
  try {
    const resp = await previewFilter({
      assetNames: sampleAssetNames(),
      filterMode: form.filterMode,
      includePatterns: form.assetIncludePatterns,
      excludePatterns: form.assetExcludePatterns
    })
    if (resp.error) {
      previewError.value = resp.error
    }
    previewResults.value = resp.results
    showPreview.value = true
  } catch (err) {
    message.error(err instanceof Error ? err.message : '预览失败')
  } finally {
    previewLoading.value = false
  }
}

function sampleAssetNames(): string[] {
  const owner = form.owner.trim() || 'owner'
  const repo = form.repo.trim() || 'repo'
  return [
    `${repo}_linux_amd64.tar.gz`,
    `${repo}_linux_arm64.tar.gz`,
    `${repo}_darwin_amd64.zip`,
    `${repo}_darwin_arm64.zip`,
    `${repo}_windows_amd64.zip`,
    `${repo}_windows_arm64.zip`,
    `${owner}-${repo}-1.0.0.apk`,
    `checksums.txt`,
    `SHA256SUMS`,
    `${repo}_linux_amd64.deb`,
    `${repo}-debug.apk`
  ]
}
</script>

<template>
  <NDrawer
    :show="show"
    width="520"
    placement="right"
    @update:show="emit('update:show', $event)"
  >
    <NDrawerContent :title="title" closable>
      <NForm class="repository-form" label-placement="top">
        <NFormItem label="Provider">
          <NSelect v-model:value="form.provider" :options="providerOptions" :disabled="ownerDisabled" />
        </NFormItem>

        <NFormItem v-if="form.provider !== 'github'" label="API Base URL">
          <NInput v-model:value="form.providerApiBaseUrl" :disabled="ownerDisabled" placeholder="https://gitlab.example.com/api/v4" />
        </NFormItem>

        <NFormItem label="Owner">
          <NInput v-model:value="form.owner" :disabled="ownerDisabled" placeholder="hashicorp" />
        </NFormItem>

        <NFormItem label="Repository">
          <NInput v-model:value="form.repo" :disabled="ownerDisabled" placeholder="terraform" />
        </NFormItem>

        <NFormItem label="GitHub Token">
          <NSelect
            v-model:value="selectedTokenId"
            :options="tokenOptions"
            placeholder="选择 Token 或使用匿名请求"
          />
        </NFormItem>

        <NFormItem label="存储目标">
          <NSelect
            v-model:value="selectedStorageIds"
            :options="storageOptions"
            multiple
            placeholder="选择一个或多个存储目标"
          />
        </NFormItem>

        <NFormItem label="代理">
          <NSelect
            v-model:value="selectedProxyId"
            :options="proxyOptions"
            placeholder="选择代理"
          />
        </NFormItem>

        <NFormItem label="启用同步">
          <NSwitch v-model:value="form.enabled" />
        </NFormItem>

        <NFormItem label="检查间隔（秒）">
          <div style="display: flex; flex-direction: column; gap: 8px; width: 100%">
            <NButtonGroup>
              <NButton
                v-for="preset in intervalPresets"
                :key="preset.value"
                size="small"
                :type="form.intervalSeconds === preset.value ? 'primary' : 'default'"
                @click="form.intervalSeconds = preset.value"
              >
                {{ preset.label }}
              </NButton>
            </NButtonGroup>
            <NInputNumber v-model:value="form.intervalSeconds" :min="300" :step="300" style="width: 100%" />
          </div>
        </NFormItem>

        <NFormItem label="Tag 过滤模式">
          <NRadioGroup v-model:value="form.tagFilterMode">
            <NRadioButton value="">不过滤</NRadioButton>
            <NRadioButton value="glob">Glob</NRadioButton>
            <NRadioButton value="regex">Regex</NRadioButton>
          </NRadioGroup>
        </NFormItem>

        <NFormItem v-if="form.tagFilterMode !== ''" label="Tag 快捷预设">
          <NSelect
            :value="[]"
            multiple
            placeholder="点击应用常见 Tag 规则"
            :options="tagFilterPresetOptions"
            @update:value="(value) => applyTagFilterPreset((value as string[]) ?? [])"
          />
        </NFormItem>

        <template v-if="form.tagFilterMode !== ''">
          <NFormItem label="Tag Include 规则">
            <NInput
              v-model:value="form.tagIncludePattern"
              type="textarea"
              placeholder="每行一条，例如：v*&#10;只同步匹配的 tag"
            />
          </NFormItem>
          <NFormItem label="Tag Exclude 规则">
            <NInput
              v-model:value="form.tagExcludePattern"
              type="textarea"
              placeholder="每行一条，例如：*-alpha&#10;排除匹配的 tag"
            />
          </NFormItem>
        </template>

        <NFormItem label="资产过滤模式">
          <NRadioGroup v-model:value="form.filterMode">
            <NRadioButton value="glob">Glob</NRadioButton>
            <NRadioButton value="regex">Regex</NRadioButton>
          </NRadioGroup>
        </NFormItem>

        <NFormItem label="资产快捷预设">
          <NSelect
            v-model:value="selectedFilterPresets"
            multiple
            clearable
            placeholder="可多选，自动合并并去重"
            :options="filterPresetOptions"
            @update:value="(value) => applyFilterPresets((value as string[]) ?? [])"
          />
        </NFormItem>

        <NFormItem label="Include 规则">
          <NInput
            v-model:value="form.assetIncludePatterns"
            type="textarea"
            placeholder="每行一条，例如：*linux*amd64*"
          />
        </NFormItem>

        <NFormItem label="Exclude 规则">
          <NInput
            v-model:value="form.assetExcludePatterns"
            type="textarea"
            placeholder="每行一条，例如：*debug*"
          />
        </NFormItem>

        <div class="filter-actions">
          <NButton secondary :loading="previewLoading" @click="handlePreview">预览匹配结果</NButton>
        </div>

        <NFormItem label="保留最近版本数">
          <NInputNumber v-model:value="form.retentionKeepLatest" :min="1" :step="1" />
        </NFormItem>
      </NForm>

      <template #footer>
        <NSpace justify="end">
          <NButton @click="emit('update:show', false)">取消</NButton>
          <NButton type="primary" :loading="saving" @click="submit">保存</NButton>
        </NSpace>
      </template>
    </NDrawerContent>
  </NDrawer>

  <NModal v-model:show="showPreview" preset="dialog" title="过滤规则预览" positive-text="关闭">
    <NAlert v-if="previewError" type="error" :show-icon="false">{{ previewError }}</NAlert>
    <div class="preview-list">
      <div v-for="item in previewResults" :key="item.name" class="preview-item">
        <NTag size="small" :type="item.matched ? 'success' : 'default'">{{ item.matched ? '匹配' : '跳过' }}</NTag>
        <span class="preview-name">{{ item.name }}</span>
      </div>
    </div>
  </NModal>
</template>

<style scoped>
.repository-form {
  padding-right: 4px;
}

.filter-actions {
  display: flex;
  justify-content: flex-end;
  margin: -8px 0 16px;
}

.preview-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 8px;
}

.preview-item {
  display: flex;
  align-items: center;
  gap: 8px;
}

.preview-name {
  font-size: 13px;
  color: #344054;
}
</style>
