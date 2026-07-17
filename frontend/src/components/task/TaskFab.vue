<script setup lang="ts">
import { computed, onMounted, onUnmounted, shallowRef } from "vue";
import {
  NBadge,
  NButton,
  NDrawer,
  NDrawerContent,
  NTag,
  NSpin,
  NEmpty,
  NSpace,
  NProgress,
  useMessage,
} from "naive-ui";
import { ListChecks, RefreshCw, X, Ban, Trash2 } from "lucide-vue-next";
import { RouterLink } from "vue-router";

import { listTasks, cancelTask, clearFailedTasks } from "@/api/tasks";
import type { Task, TaskStatus } from "@/types/task";

// 自管任务列表：定时刷新活跃任务（pending/running/failed），不依赖 TasksView 的 store
const activeTasks = shallowRef<Task[]>([]);
const loading = shallowRef(false);
const error = shallowRef<string | null>(null);
const showDrawer = shallowRef(false);
let refreshTimer: number | undefined;

const message = useMessage();
// 正在取消中的任务 ID 集合（防止重复提交）
const cancelingTasks = shallowRef<Set<number>>(new Set());
// 正在清理失败任务的 loading
const clearingFailed = shallowRef(false);

// 活跃任务数（badge 显示值）
const activeCount = computed(() => activeTasks.value.length);
const runningCount = computed(
  () => activeTasks.value.filter((t) => t.status === "running").length,
);
const pendingCount = computed(
  () => activeTasks.value.filter((t) => t.status === "pending").length,
);
const failedCount = computed(
  () => activeTasks.value.filter((t) => t.status === "failed").length,
);

// 是否有任务在执行（决定按钮是否高亮脉动）
const hasRunning = computed(() => runningCount.value > 0);

async function refresh() {
  loading.value = true;
  error.value = null;
  try {
    // 并行拉取运行中 / 排队中 / 失败任务，合并按 updatedAt 倒序，最多取 50 条
    const [runningResp, pendingResp, failedResp] = await Promise.all([
      listTasks({ status: "running", pageSize: 50 }),
      listTasks({ status: "pending", pageSize: 50 }),
      listTasks({ status: "failed", pageSize: 20 }),
    ]);
    const merged = [
      ...runningResp.items,
      ...pendingResp.items,
      ...failedResp.items,
    ];
    merged.sort(
      (a, b) =>
        new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
    );
    // 去重（同一任务可能出现在不同状态查询结果中）
    const seen = new Set<number>();
    activeTasks.value = merged
      .filter((t) => {
        if (seen.has(t.id)) return false;
        seen.add(t.id);
        return true;
      })
      .slice(0, 50);
  } catch (err) {
    error.value = err instanceof Error ? err.message : "任务刷新失败";
  } finally {
    loading.value = false;
  }
}

// 停止指定任务（running 中断下载，pending 标记跳过）
async function stopTask(task: Task) {
  if (cancelingTasks.value.has(task.id)) return;
  const next = new Set(cancelingTasks.value);
  next.add(task.id);
  cancelingTasks.value = next;
  try {
    await cancelTask(task.id);
    message.success("已发送停止信号");
    await refresh();
  } catch (err) {
    message.error(err instanceof Error ? err.message : "停止任务失败");
  } finally {
    const done = new Set(cancelingTasks.value);
    done.delete(task.id);
    cancelingTasks.value = done;
  }
}

// 清理所有失败状态的任务及关联日志
async function clearFailed() {
  if (clearingFailed.value) return;
  clearingFailed.value = true;
  try {
    const resp = await clearFailedTasks();
    message.success(`已清理 ${resp.deleted} 个失败任务`);
    await refresh();
  } catch (err) {
    message.error(err instanceof Error ? err.message : "清理失败任务失败");
  } finally {
    clearingFailed.value = false;
  }
}

onMounted(() => {
  void refresh();
  // 每 3 秒静默刷新活跃任务状态
  refreshTimer = window.setInterval(() => {
    void refresh();
  }, 3000);
});

onUnmounted(() => {
  if (refreshTimer) {
    window.clearInterval(refreshTimer);
  }
});

function statusTagType(status: TaskStatus) {
  if (status === "succeeded") return "success";
  if (status === "failed") return "error";
  if (status === "running") return "warning";
  if (status === "pending") return "info";
  return "default";
}

function statusLabel(status: TaskStatus) {
  const map: Record<string, string> = {
    pending: "排队中",
    running: "运行中",
    succeeded: "成功",
    failed: "失败",
    canceled: "已取消",
  };
  return map[status] ?? status;
}

function taskTypeLabel(type: string) {
  const labels: Record<string, string> = {
    check_release: "检查版本",
    check_all_releases: "全量检查",
    sync_release: "同步版本",
    sync_all_releases: "全量同步",
    download_asset: "下载文件",
    cleanup_release: "保留清理",
  };
  return labels[type] ?? type;
}

function formatTime(value: string | null) {
  if (!value) return "-";
  return new Date(value).toLocaleTimeString();
}

// 是否有真实下载进度数据（已知总大小或已下载字节 > 0）
function hasDownloadProgress(task: Task): boolean {
  return task.totalBytes > 0 || task.downloadedBytes > 0;
}

// 进度百分比：仅在已知总大小时计算真实百分比，未知时返回 0（配合 processing 不定态）
function progressPercent(task: Task): number {
  if (task.totalBytes > 0) {
    return Math.min(
      100,
      Math.round((task.downloadedBytes / task.totalBytes) * 100),
    );
  }
  return 0;
}

// 进度标签：已知总大小显示 已下载/总大小 (百分比)，未知时只显示已下载
function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let idx = 0;
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024;
    idx++;
  }
  return idx === 0
    ? `${value} ${units[idx]}`
    : `${value.toFixed(2)} ${units[idx]}`;
}

function progressLabel(task: Task): string {
  const downloaded = formatBytes(task.downloadedBytes);
  if (task.totalBytes > 0) {
    return `${downloaded} / ${formatBytes(task.totalBytes)} (${progressPercent(task)}%)`;
  }
  return task.downloadedBytes > 0 ? `${downloaded} 已下载` : "";
}

// 排队位置（按创建时间升序）
const pendingPosition = computed(() => {
  const map = new Map<number, number>();
  const pending = activeTasks.value
    .filter((t) => t.status === "pending")
    .sort(
      (a, b) =>
        new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime(),
    );
  pending.forEach((t, idx) => map.set(t.id, idx + 1));
  return map;
});
</script>

<template>
  <!-- 悬浮按钮：固定右下角 -->
  <button
    class="task-fab"
    :class="{ 'is-running': hasRunning }"
    title="任务状态"
    @click="showDrawer = true"
  >
    <NBadge :value="activeCount" :max="99" type="info" :show="activeCount > 0">
      <ListChecks :size="22" />
    </NBadge>
  </button>

  <!-- 任务抽屉 -->
  <NDrawer v-model:show="showDrawer" :width="520" placement="right">
    <NDrawerContent :native-scrollbar="true">
      <template #header>
        <NSpace align="center" :size="12">
          <ListChecks :size="18" />
          <strong>任务状态</strong>
          <NTag v-if="runningCount > 0" size="small" type="warning"
            >运行中 {{ runningCount }}</NTag
          >
          <NTag v-if="pendingCount > 0" size="small" type="info"
            >排队中 {{ pendingCount }}</NTag
          >
          <NTag v-if="failedCount > 0" size="small" type="error"
            >失败 {{ failedCount }}</NTag
          >
          <NButton
            v-if="failedCount > 0"
            size="tiny"
            type="error"
            ghost
            :loading="clearingFailed"
            @click="clearFailed"
          >
            <template #icon><Trash2 :size="12" /></template>
            清理
          </NButton>
        </NSpace>
      </template>

      <NSpace justify="space-between" style="margin-bottom: 12px">
        <NButton size="small" secondary :loading="loading" @click="refresh">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </NButton>
        <RouterLink to="/tasks" class="goto-tasks">
          <NButton size="small" tertiary>查看全部任务</NButton>
        </RouterLink>
      </NSpace>

      <NSpin
        v-if="loading && activeTasks.length === 0"
        :show="true"
        style="padding: 24px"
      />

      <NEmpty
        v-else-if="activeTasks.length === 0"
        description="当前无活跃任务"
        style="padding: 32px"
      />

      <div v-else class="task-list">
        <div
          v-for="task in activeTasks"
          :key="task.id"
          class="task-item"
          :class="{ 'is-running': task.status === 'running' }"
        >
          <div class="task-item-header">
            <NTag size="small" :type="statusTagType(task.status)">
              {{ statusLabel(task.status) }}
            </NTag>
            <span
              v-if="task.status === 'pending' && pendingPosition.has(task.id)"
              class="queue-pos"
            >
              队列第 {{ pendingPosition.get(task.id) }} 位
            </span>
            <span class="task-type">{{ taskTypeLabel(task.type) }}</span>
            <span class="task-time">{{
              formatTime(task.startedAt || task.createdAt)
            }}</span>
            <NButton
              v-if="task.status === 'running' || task.status === 'pending'"
              size="tiny"
              quaternary
              :loading="cancelingTasks.has(task.id)"
              @click="stopTask(task)"
            >
              <template #icon><Ban :size="12" /></template>
              停止
            </NButton>
          </div>
          <div class="task-item-body">
            <span class="task-repo">{{
              task.repositoryName || `#${task.repositoryId}`
            }}</span>
            <span v-if="task.releaseTag" class="task-tag">{{
              task.releaseTag
            }}</span>
            <span v-if="task.assetName" class="task-asset">{{
              task.assetName
            }}</span>
          </div>
          <div v-if="task.status === 'running'" class="task-progress">
            <div class="progress-line">
              <NProgress
                type="line"
                :percentage="
                  hasDownloadProgress(task) ? progressPercent(task) : 100
                "
                :show-indicator="false"
                :height="6"
                status="warning"
                :processing="!hasDownloadProgress(task)"
              />
              <span v-if="hasDownloadProgress(task)" class="progress-label">{{
                progressLabel(task)
              }}</span>
            </div>
          </div>
          <div v-if="task.errorMessage" class="task-error">
            {{ task.errorMessage }}
          </div>
        </div>
      </div>

      <template #footer>
        <NButton quaternary @click="showDrawer = false">
          <template #icon><X :size="16" /></template>
          关闭
        </NButton>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.task-fab {
  position: fixed;
  right: 24px;
  bottom: 24px;
  width: 52px;
  height: 52px;
  border: none;
  border-radius: 14px;
  background: #1f6feb;
  color: #ffffff;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 12px rgba(31, 111, 235, 0.35);
  transition:
    transform 0.15s,
    box-shadow 0.15s;
  z-index: 1000;
}

.task-fab:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(31, 111, 235, 0.45);
}

/* 运行中脉动提示 */
.task-fab.is-running {
  animation: fab-pulse 1.5s ease-in-out infinite;
}

@keyframes fab-pulse {
  0%,
  100% {
    box-shadow: 0 4px 12px rgba(31, 111, 235, 0.35);
  }
  50% {
    box-shadow: 0 4px 20px rgba(31, 111, 235, 0.65);
  }
}

.task-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.task-item {
  padding: 12px;
  border: 1px solid #e4e7ec;
  border-radius: 8px;
  background: #ffffff;
}

.task-item.is-running {
  border-color: #f0a000;
  background: #fff9f0;
}

.task-item-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.queue-pos {
  color: #1f6feb;
  font-size: 12px;
}

.task-type {
  color: #475467;
  font-size: 13px;
}

.task-time {
  margin-left: auto;
  color: #8c8c8c;
  font-size: 12px;
}

.task-item-body {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.task-repo {
  color: #101828;
  font-weight: 600;
}

.task-tag {
  color: #667085;
}

.task-asset {
  color: #667085;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 200px;
}

.task-progress {
  margin-top: 8px;
}

.progress-line {
  display: flex;
  align-items: center;
  gap: 8px;
}

.progress-label {
  font-size: 12px;
  color: #475467;
  white-space: nowrap;
}

.task-error {
  margin-top: 6px;
  color: #d03030;
  font-size: 12px;
  word-break: break-all;
}

.goto-tasks {
  text-decoration: none;
}
</style>
