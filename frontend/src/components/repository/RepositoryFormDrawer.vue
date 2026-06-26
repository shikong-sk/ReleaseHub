<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from 'vue'
import {
  NAlert,
  NButton,
  NDrawer,
  NDrawerContent,
  NForm,
  NFormItem,
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

const tokensStore = useTokensStore()
const storagesStore = useStoragesStore()
const proxiesStore = useProxiesStore()

const selectedTokenId = computed<number | undefined>({
  get: () => form.githubTokenId ?? undefined,
  set: (val: number | undefined) => {
    form.githubTokenId = val ?? null
  }
})

const selectedStorageId = computed<number | undefined>({
  get: () => form.storageId ?? undefined,
  set: (val: number | undefined) => {
    form.storageId = val ?? null
  }
})

const selectedProxyId = computed<number | undefined>({
  get: () => form.proxyId ?? undefined,
  set: (val: number | undefined) => {
    form.proxyId = val ?? null
  }
})

const form = reactive<RepositoryPayload>({
  owner: '',
  repo: '',
  githubTokenId: null,
  storageId: null,
  proxyId: null,
  enabled: true,
  intervalSeconds: 1800,
  filterMode: 'glob',
  assetIncludePatterns: '',
  assetExcludePatterns: '',
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

const storageOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = [
    { label: '默认存储', value: 0 }
  ]
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
  form.owner = props.repository?.owner ?? ''
  form.repo = props.repository?.repo ?? ''
  form.githubTokenId = props.repository?.githubTokenId ?? null
  form.storageId = props.repository?.storageId ?? null
  form.proxyId = props.repository?.proxyId ?? null
  form.enabled = props.repository?.enabled ?? true
  form.intervalSeconds = props.repository?.intervalSeconds ?? 1800
  form.filterMode = (props.repository?.filterMode ?? 'glob') as RepositoryFilterMode
  form.assetIncludePatterns = props.repository?.assetIncludePatterns ?? ''
  form.assetExcludePatterns = props.repository?.assetExcludePatterns ?? ''
  form.retentionKeepLatest = props.repository?.retentionKeepLatest ?? 5
}

function submit() {
  const tokenId = form.githubTokenId
  const storageId = form.storageId
  const proxyId = form.proxyId
  emit('submit', {
    owner: form.owner.trim(),
    repo: form.repo.trim(),
    githubTokenId: tokenId === 0 || tokenId === null ? null : tokenId,
    storageId: storageId === 0 || storageId === null ? null : storageId,
    proxyId: proxyId === 0 || proxyId === null ? null : proxyId,
    enabled: form.enabled,
    intervalSeconds: form.intervalSeconds,
    filterMode: form.filterMode,
    assetIncludePatterns: form.assetIncludePatterns.trim(),
    assetExcludePatterns: form.assetExcludePatterns.trim(),
    retentionKeepLatest: form.retentionKeepLatest
  })
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
          <NSelect v-model:value="form.provider" :options="providerOptions" />
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
            v-model:value="selectedStorageId"
            :options="storageOptions"
            placeholder="选择存储目标"
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
          <NInputNumber v-model:value="form.intervalSeconds" :min="300" :step="300" />
        </NFormItem>

        <NFormItem label="资产过滤模式">
          <NRadioGroup v-model:value="form.filterMode">
            <NRadioButton value="glob">Glob</NRadioButton>
            <NRadioButton value="regex">Regex</NRadioButton>
          </NRadioGroup>
        </NFormItem>

        <NFormItem label="Include 规则">
          <div class="filter-row">
            <NInput
              v-model:value="form.assetIncludePatterns"
              type="textarea"
              placeholder="例如：*linux*amd64*"
            />
            <NButton size="small" secondary :loading="previewLoading" @click="handlePreview">预览</NButton>
          </div>
        </NFormItem>

        <NFormItem label="Exclude 规则">
          <NInput
            v-model:value="form.assetExcludePatterns"
            type="textarea"
            placeholder="例如：*debug*"
          />
        </NFormItem>

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

.filter-row {
  display: flex;
  gap: 8px;
  align-items: flex-start;
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
