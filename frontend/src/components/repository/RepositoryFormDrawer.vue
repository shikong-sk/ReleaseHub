<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import {
  NButton,
  NDrawer,
  NDrawerContent,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NRadioButton,
  NRadioGroup,
  NSpace,
  NSwitch
} from 'naive-ui'

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

const form = reactive<RepositoryPayload>({
  owner: '',
  repo: '',
  enabled: true,
  intervalSeconds: 1800,
  filterMode: 'glob',
  assetIncludePatterns: '',
  assetExcludePatterns: '',
  retentionKeepLatest: 5
})

const title = computed(() => (props.mode === 'create' ? '新增 GitHub 仓库' : '编辑仓库'))
const ownerDisabled = computed(() => props.mode === 'edit')

watch(
  () => [props.show, props.repository, props.mode] as const,
  () => {
    if (!props.show) {
      return
    }
    resetForm()
  },
  { immediate: true }
)

function resetForm() {
  form.owner = props.repository?.owner ?? ''
  form.repo = props.repository?.repo ?? ''
  form.enabled = props.repository?.enabled ?? true
  form.intervalSeconds = props.repository?.intervalSeconds ?? 1800
  form.filterMode = (props.repository?.filterMode ?? 'glob') as RepositoryFilterMode
  form.assetIncludePatterns = props.repository?.assetIncludePatterns ?? ''
  form.assetExcludePatterns = props.repository?.assetExcludePatterns ?? ''
  form.retentionKeepLatest = props.repository?.retentionKeepLatest ?? 5
}

function submit() {
  emit('submit', {
    owner: form.owner.trim(),
    repo: form.repo.trim(),
    enabled: form.enabled,
    intervalSeconds: form.intervalSeconds,
    filterMode: form.filterMode,
    assetIncludePatterns: form.assetIncludePatterns.trim(),
    assetExcludePatterns: form.assetExcludePatterns.trim(),
    retentionKeepLatest: form.retentionKeepLatest
  })
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
        <NFormItem label="Owner">
          <NInput v-model:value="form.owner" :disabled="ownerDisabled" placeholder="hashicorp" />
        </NFormItem>

        <NFormItem label="Repository">
          <NInput v-model:value="form.repo" :disabled="ownerDisabled" placeholder="terraform" />
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
          <NInput
            v-model:value="form.assetIncludePatterns"
            type="textarea"
            placeholder="例如：*linux*amd64*"
          />
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
</template>

<style scoped>
.repository-form {
  padding-right: 4px;
}
</style>
