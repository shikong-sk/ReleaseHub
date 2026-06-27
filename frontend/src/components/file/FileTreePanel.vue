<script setup lang="ts">
import { h, shallowRef, watch } from 'vue'
import {
  NButton,
  NPopconfirm,
  NTag,
  NTree,
  NSpin,
  NEmpty,
  useMessage
} from 'naive-ui'
import type { TreeOption } from 'naive-ui'
import { Download, Trash2 } from 'lucide-vue-next'

import { getRepositoryFileTree, assetFileURL } from '@/api/files'
import { deleteAsset } from '@/api/releases'
import type { FileTreeNode } from '@/types/file'

const props = defineProps<{
  tree: FileTreeNode[]
  loading: boolean
  canWrite: boolean
  storageId?: number | null
}>()

const emit = defineEmits<{
  refresh: []
}>()

const message = useMessage()

// 本地可变树：保留已懒加载的 children，避免 computed 重算时丢失
const localTree = shallowRef<TreeOption[]>([])

// 全量元数据索引：包含顶层 + 懒加载的所有节点，用于 findRaw 查找
const rawMap = new Map<string, FileTreeNode>()

// 将节点及其子节点全部注册到 rawMap
function registerRawNodes(nodes: FileTreeNode[] | null | undefined) {
  if (!nodes) return
  for (const node of nodes) {
    rawMap.set(node.key, node)
    if (node.children) registerRawNodes(node.children)
  }
}

watch(
  () => [props.tree, props.storageId] as const,
  ([rawTree, storageId]) => {
    // 顶层数据变化时重建 rawMap
    rawMap.clear()
    registerRawNodes(rawTree)

    const source = storageId == null
      ? rawTree
      : (rawTree ?? []).filter((n) => n.storageId === storageId)
    localTree.value = mergeIncoming(source, localTree.value)
  },
  { immediate: true }
)

// 合并后端新数据到本地树，保留已加载的子节点
function mergeIncoming(incoming: FileTreeNode[], existing: TreeOption[]): TreeOption[] {
  const existingMap = indexOptions(existing)
  return incoming.map((node) => {
    const opt: TreeOption = { key: node.key, label: node.label, isLeaf: node.isLeaf }
    const prev = existingMap.get(String(node.key))
    if (prev?.children && prev.children.length > 0) {
      opt.children = prev.children
    } else if (node.children && node.children.length > 0) {
      opt.children = mergeIncoming(node.children, prev?.children ?? [])
    } else if (!node.isLeaf && node.key.startsWith('repo-')) {
      // 仓库尚未加载子节点，标记为异步加载
      opt.children = undefined
    }
    return opt
  })
}

function indexOptions(nodes: TreeOption[]): Map<string, TreeOption> {
  const map = new Map<string, TreeOption>()
  function walk(list: TreeOption[]) {
    for (const n of list) {
      map.set(String(n.key), n)
      if (n.children) walk(n.children)
    }
  }
  walk(nodes)
  return map
}

// 从 rawMap 查找节点元数据
function findRaw(key: string): FileTreeNode | null {
  return rawMap.get(key) ?? null
}

// 节点前缀图标
function renderPrefix({ option }: { option: TreeOption }): string {
  const key = String(option.key)
  if (key.startsWith('storage-')) return '💾'
  if (key.startsWith('repo-')) return '📁'
  if (key.startsWith('release-')) return '🏷️'
  // 文件节点根据状态显示不同图标
  if (key.startsWith('asset-')) {
    const raw = findRaw(key)
    if (raw?.status === 'pending') return '⏳'
    if (raw?.status === 'downloading') return '⬇️'
    if (raw?.status === 'failed') return '❌'
    return '📄'
  }
  return ''
}

// 节点标签：文件名左对齐，元信息跟随
function renderLabel({ option }: { option: TreeOption }) {
  const key = String(option.key)

  // 仓库节点：名称 + 文件数标签
  if (key.startsWith('repo-')) {
    const raw = findRaw(key)
    if (raw?.fileCount != null) {
      return h('span', { style: 'display: inline-flex; align-items: center; gap: 8px' }, [
        h('span', raw.label),
        raw.fileCount > 0
          ? h(NTag, { size: 'small', type: 'info' }, { default: () => `${raw.fileCount} 文件` })
          : h(NTag, { size: 'small', type: 'default' }, { default: () => '暂无文件' })
      ])
    }
  }

  // 版本节点：正在同步的显示状态标签
  if (key.startsWith('release-')) {
    const raw = findRaw(key)
    // label 中已经包含 "(同步中)" 后缀
    if (raw?.label?.includes('(同步中)')) {
      return h('span', { style: 'display: inline-flex; align-items: center; gap: 8px' }, [
        h('span', raw.label),
        h(NTag, { size: 'small', type: 'warning' }, { default: () => '同步中' })
      ])
    }
    return raw?.label ?? option.label
  }

  // 文件节点：文件名 + 大小 + SHA256 摘要
  if (key.startsWith('asset-')) {
    const raw = findRaw(key)
    if (raw?.size != null) {
      const children = [
        h('span', { style: 'overflow: hidden; text-overflow: ellipsis; white-space: nowrap' }, raw.label)
      ]

      // 非 verified/downloaded 状态时显示状态标签代替大小
      if (raw.status && raw.status !== 'verified' && raw.status !== 'downloaded') {
        const tagType = raw.status === 'downloading' ? 'info'
          : raw.status === 'pending' ? 'warning'
          : raw.status === 'failed' ? 'error' : 'default'
        children.push(
          h(NTag, { size: 'small', type: tagType as any, flexShrink: 0 },
            { default: () => statusText(raw.status!) })
        )
      } else {
        children.push(
          h(NTag, { size: 'small', flexShrink: 0 }, { default: () => formatBytes(raw.size!) })
        )
      }

      if (raw.sha256) {
        children.push(
          h('span', {
            style: 'font-size: 11px; color: #667085; max-width: 120px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex-shrink: 0',
            title: raw.sha256
          }, raw.sha256.slice(0, 12) + '...')
        )
      }

      return h('span', { style: 'display: inline-flex; align-items: center; gap: 8px; min-width: 0; flex: 1' }, children)
    }
  }

  return option.label as string
}

// 节点尾部：文件叶节点显示操作按钮，右对齐
function renderSuffix({ option }: { option: TreeOption }) {
  const key = String(option.key)
  if (!key.startsWith('asset-')) return null
  const raw = findRaw(key)
  if (!raw?.assetId) return null

  // 正在下载或等待下载的不显示操作按钮
  if (raw.status === 'pending' || raw.status === 'downloading') return null

  const buttons: any[] = []

  // 已验证/已下载的显示下载按钮
  if (raw.status === 'verified' || raw.status === 'downloaded') {
    buttons.push(
      h(NButton, {
        size: 'tiny', type: 'primary', secondary: true, tag: 'a',
        href: assetFileURL(raw.assetId)
      }, { default: () => '下载' })
    )
  }

  // failed 和 verified/downloaded 都可以删除
  if (props.canWrite) {
    buttons.push(
      h(NPopconfirm, { positiveText: "确定", negativeText: "取消",
        onPositiveClick: () => handleDelete(raw.assetId!, raw.label)
      }, {
        trigger: () => h(NButton, {
          size: 'tiny', type: 'error', secondary: true
        }, { icon: () => h(Trash2, { size: 12 }) }),
        default: () => `删除 ${raw.label}？`
      })
    )
  }

  return buttons.length > 0
    ? h('div', { style: 'display: flex; gap: 6px; margin-left: auto; flex-shrink: 0' }, buttons)
    : null
}

function statusText(status: string): string {
  const map: Record<string, string> = {
    pending: '待下载',
    downloading: '下载中',
    downloaded: '已下载',
    verified: '已验证',
    failed: '失败',
    skipped: '已跳过',
    deleted: '已删除'
  }
  return map[status] ?? status
}

// 懒加载：展开仓库节点时请求 版本→文件 子树
async function onLoad(node: TreeOption) {
  const raw = findRaw(String(node.key))
  if (!raw?.repositoryId) return

  try {
    const result = await getRepositoryFileTree(raw.repositoryId)
    // 将懒加载返回的子树节点注册到 rawMap，使 findRaw 能查到
    registerRawNodes(result.tree)

    if (result.tree.length > 0) {
      node.children = mergeIncoming(result.tree, [])
      // shallowRef 不会追踪深层属性变化，必须替换顶层引用才能触发 NTree 重新渲染
      localTree.value = [...localTree.value]
    } else {
      node.isLeaf = true
    }
  } catch {
    message.error('加载仓库文件失败')
  }
}

async function handleDelete(assetId: number, name: string) {
  try {
    await deleteAsset(assetId)
    message.success(`${name} 已删除`)
    emit('refresh')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除失败')
  }
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}
</script>

<template>
  <NSpin :show="loading">
    <NTree
      v-if="localTree.length > 0"
      :data="localTree"
      :prefix="renderPrefix"
      :render-label="renderLabel"
      :render-suffix="renderSuffix"
      :on-load="onLoad"
      :virtual-scroll="true"
      expand-on-click
      :default-expand-all="false"
      block-node
      style="min-height: 200px; max-height: calc(90vh - 120px)"
    />
    <NEmpty v-else-if="!loading" description="暂无已同步文件" />
  </NSpin>
</template>

<style scoped>
/* block-node 使 content 占满整行，suffix 用 margin-left: auto 右对齐 */
:deep(.n-tree-node-content) {
  display: flex !important;
  align-items: center !important;
}

:deep(.n-tree-node-content__suffix) {
  margin-left: auto !important;
  flex-shrink: 0 !important;
}
</style>
