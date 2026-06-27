<script setup lang="ts">
import { computed, h, shallowRef } from 'vue'
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

import { getRepositoryFileTree } from '@/api/files'
import { assetFileURL } from '@/api/files'
import { deleteAsset } from '@/api/releases'
import type { FileTreeNode } from '@/types/file'

const props = defineProps<{
  tree: FileTreeNode[]
  loading: boolean
  canWrite: boolean
  // 可选：只展示指定存储 ID 下的节点
  storageId?: number | null
}>()

const emit = defineEmits<{
  refresh: []
}>()

const message = useMessage()
const loadingKeys = shallowRef<Set<string>>(new Set())

// 按存储 ID 过滤
const filteredTree = computed(() => {
  if (props.storageId == null) return props.tree
  const targetKey = `storage-${props.storageId}`
  return props.tree.filter((node) => node.key === targetKey)
})

// 把 FileTreeNode 转成 naive-ui TreeOption，仓库层标记为异步加载
function toTreeOptions(nodes: FileTreeNode[]): TreeOption[] {
  return nodes.map((node) => {
    const opt: TreeOption = {
      key: node.key,
      label: node.label,
      isLeaf: node.isLeaf,
    }
    if (node.children && node.children.length > 0) {
      opt.children = toTreeOptions(node.children)
    } else if (!node.isLeaf && node.key.startsWith('repo-')) {
      // 仓库层没有子节点时标记为异步加载
      opt.children = undefined
    }
    return opt
  })
}

const treeOptions = computed(() => toTreeOptions(filteredTree.value))

// 节点前缀渲染：根据 key 前缀显示图标
function renderPrefix({ option }: { option: TreeOption }): string {
  const raw = findRawNode(option.key as string)
  if (!raw) return ''

  if (raw.key.startsWith('storage-')) {
    return '\uD83D\uDCBE'
  }
  if (raw.key.startsWith('repo-')) {
    return '\uD83D\uDCC1'
  }
  if (raw.key.startsWith('release-')) {
    return '\uD83C\uDFF7\uFE0F'
  }
  if (raw.key.startsWith('asset-')) {
    return '\uD83D\uDCC4'
  }
  return ''
}

// 节点标签渲染：仓库显示文件数、版本显示子文件数、文件显示大小
function renderLabel({ option }: { option: TreeOption }) {
  const raw = findRawNode(option.key as string)
  if (!raw) return option.label as string

  if (raw.key.startsWith('repo-') && raw.fileCount) {
    return h('span', [
      h('span', raw.label),
      h(NTag, { size: 'small', type: 'info', style: 'margin-left: 8px' }, {
        default: () => `${raw.fileCount} 文件`
      })
    ])
  }

  if (raw.key.startsWith('asset-') && raw.size != null) {
    return h('span', { style: 'display: flex; align-items: center; gap: 8px' }, [
      h('span', raw.label),
      h(NTag, { size: 'small' }, { default: () => formatBytes(raw.size!) }),
      raw.sha256
        ? h('span', {
            style: 'font-size: 11px; color: #667085; max-width: 120px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap',
            title: raw.sha256
          }, raw.sha256.slice(0, 12) + '...')
        : null
    ])
  }

  return raw.label
}

// 节点尾部渲染：文件叶节点显示操作按钮
function renderSuffix({ option }: { option: TreeOption }) {
  const raw = findRawNode(option.key as string)
  if (!raw || !raw.key.startsWith('asset-') || !raw.assetId) return null

  const buttons = [
    h(NButton, {
      size: 'tiny',
      type: 'primary',
      secondary: true,
      tag: 'a',
      href: assetFileURL(raw.assetId)
    }, { default: () => '下载' })
  ]

  if (props.canWrite) {
    buttons.push(
      h(NPopconfirm, {
        onPositiveClick: () => handleDelete(raw.assetId!, raw.label)
      }, {
        trigger: () => h(NButton, {
          size: 'tiny',
          type: 'error',
          secondary: true
        }, {
          icon: () => h(Trash2, { size: 12 })
        }),
        default: () => `删除 ${raw.label}？`
      })
    )
  }

  return h('div', { style: 'display: flex; gap: 6px' }, buttons)
}

// 懒加载：展开仓库节点时请求 版本→文件 子树
async function onLoad(node: TreeOption) {
  const raw = findRawNode(node.key as string)
  if (!raw || !raw.key.startsWith('repo-') || !raw.repositoryId) return

  loadingKeys.value = new Set([...loadingKeys.value, node.key as string])
  try {
    const result = await getRepositoryFileTree(raw.repositoryId)
    if (result.tree.length > 0) {
      node.children = toTreeOptions(result.tree)
      // 把原始节点数据也补上，方便后续渲染
      raw.children = result.tree
    } else {
      node.isLeaf = true
      raw.isLeaf = true
    }
  } catch {
    message.error('加载仓库文件失败')
  } finally {
    const next = new Set(loadingKeys.value)
    next.delete(node.key as string)
    loadingKeys.value = next
  }
}

// 在树中查找原始 FileTreeNode
function findRawNode(key: string, nodes?: FileTreeNode[]): FileTreeNode | null {
  const source = nodes ?? props.tree
  for (const node of source) {
    if (node.key === key) return node
    if (node.children) {
      const found = findRawNode(key, node.children)
      if (found) return found
    }
  }
  return null
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
      v-if="treeOptions.length > 0"
      :data="treeOptions"
      :prefix="renderPrefix"
      :render-label="renderLabel"
      :render-suffix="renderSuffix"
      :on-load="onLoad"
      :virtual-scroll="true"
      block-line
      expand-on-click
      :default-expand-all="false"
      style="min-height: 200px"
    />
    <NEmpty v-else-if="!loading" description="暂无已同步文件" />
  </NSpin>
</template>
