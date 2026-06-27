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

watch(
  () => [props.tree, props.storageId] as const,
  ([rawTree, storageId]) => {
    const source = storageId == null
      ? rawTree
      : rawTree.filter((n) => n.key === `storage-${storageId}`)
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

// 在原始 props.tree 中查找节点（用于读取 fileCount/size/sha256 等元数据）
function findRaw(key: string): FileTreeNode | null {
  for (const node of props.tree) {
    const found = searchRaw(node, key)
    if (found) return found
  }
  return null
}

function searchRaw(node: FileTreeNode, key: string): FileTreeNode | null {
  if (node.key === key) return node
  if (node.children) {
    for (const child of node.children) {
      const found = searchRaw(child, key)
      if (found) return found
    }
  }
  return null
}

// 节点前缀图标
function renderPrefix({ option }: { option: TreeOption }): string {
  const key = String(option.key)
  if (key.startsWith('storage-')) return '💾'
  if (key.startsWith('repo-')) return '📁'
  if (key.startsWith('release-')) return '🏷️'
  if (key.startsWith('asset-')) return '📄'
  return ''
}

// 节点标签：文件名左对齐，元信息跟随
function renderLabel({ option }: { option: TreeOption }) {
  const key = String(option.key)

  // 仓库节点：名称 + 文件数标签
  if (key.startsWith('repo-')) {
    const raw = findRaw(key)
    if (raw?.fileCount) {
      return h('span', { style: 'display: inline-flex; align-items: center; gap: 8px' }, [
        h('span', raw.label),
        h(NTag, { size: 'small', type: 'info' }, {
          default: () => `${raw.fileCount} 文件`
        })
      ])
    }
  }

  // 文件节点：文件名 + 大小 + SHA256 摘要
  if (key.startsWith('asset-')) {
    const raw = findRaw(key)
    if (raw?.size != null) {
      return h('span', { style: 'display: inline-flex; align-items: center; gap: 8px; min-width: 0; flex: 1' }, [
        h('span', { style: 'overflow: hidden; text-overflow: ellipsis; white-space: nowrap' }, raw.label),
        h(NTag, { size: 'small', flexShrink: 0 }, { default: () => formatBytes(raw.size!) }),
        raw.sha256
          ? h('span', {
              style: 'font-size: 11px; color: #667085; max-width: 120px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex-shrink: 0',
              title: raw.sha256
            }, raw.sha256.slice(0, 12) + '...')
          : null
      ])
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

  const buttons = [
    h(NButton, {
      size: 'tiny', type: 'primary', secondary: true, tag: 'a',
      href: assetFileURL(raw.assetId)
    }, { default: () => '下载' })
  ]

  if (props.canWrite) {
    buttons.push(
      h(NPopconfirm, {
        onPositiveClick: () => handleDelete(raw.assetId!, raw.label)
      }, {
        trigger: () => h(NButton, {
          size: 'tiny', type: 'error', secondary: true
        }, { icon: () => h(Trash2, { size: 12 }) }),
        default: () => `删除 ${raw.label}？`
      })
    )
  }

  return h('div', { style: 'display: flex; gap: 6px; margin-left: auto; flex-shrink: 0' }, buttons)
}

// 懒加载：展开仓库节点时请求 版本→文件 子树
async function onLoad(node: TreeOption) {
  const raw = findRaw(String(node.key))
  if (!raw?.repositoryId) return

  try {
    const result = await getRepositoryFileTree(raw.repositoryId)
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
      style="min-height: 200px; max-height: 70vh"
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
