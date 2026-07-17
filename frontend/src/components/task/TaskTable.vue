<script setup lang="ts">
import { computed, h } from "vue";
import { NButton, NDataTable, NTag } from "naive-ui";
import type { DataTableColumns } from "naive-ui";
import { ScrollText } from "lucide-vue-next";

import type { Task } from "@/types/task";

const props = defineProps<{
  tasks: Task[];
  loading: boolean;
}>();

const emit = defineEmits<{
  viewLogs: [task: Task];
}>();

// 计算排队中任务的位置（按创建时间升序，即先入队的在前）
const pendingPosition = computed(() => {
  const map = new Map<number, number>();
  const pending = props.tasks
    .filter((t) => t.status === "pending")
    .sort(
      (a, b) =>
        new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime(),
    );
  pending.forEach((t, idx) => map.set(t.id, idx + 1));
  return map;
});

const columns = computed<DataTableColumns<Task>>(() => [
  {
    title: "ID",
    key: "id",
    width: 80,
  },
  {
    title: "类型",
    key: "type",
    width: 140,
    render: (row) => taskTypeLabel(row.type),
  },
  {
    title: "状态",
    key: "status",
    width: 130,
    render: (row) =>
      h(
        NTag,
        {
          type: statusTagType(row.status),
        },
        { default: () => statusLabel(row.status) },
      ),
  },
  {
    title: "队列位置",
    key: "queuePosition",
    width: 110,
    render: (row) => {
      if (row.status !== "pending") return "-";
      const pos = pendingPosition.value.get(row.id);
      return pos ? `第 ${pos} 位` : "-";
    },
  },
  {
    title: "仓库",
    key: "repositoryName",
    width: 220,
    ellipsis: { tooltip: true },
    render: (row) => row.repositoryName || fallbackID(row.repositoryId),
  },
  {
    title: "文件",
    key: "assetName",
    width: 260,
    ellipsis: { tooltip: true },
    render: (row) => row.assetName || fallbackID(row.assetId),
  },
  {
    title: "存储位置",
    key: "storagePath",
    width: 320,
    ellipsis: { tooltip: true },
    render: (row) => row.storagePath || "-",
  },
  {
    title: "开始时间",
    key: "startedAt",
    width: 190,
    render: (row) => formatTime(row.startedAt),
  },
  {
    title: "结束时间",
    key: "finishedAt",
    width: 190,
    render: (row) => formatTime(row.finishedAt),
  },
  {
    title: "错误",
    key: "errorMessage",
    ellipsis: {
      tooltip: true,
    },
    render: (row) => row.errorMessage || "-",
  },
  {
    title: "操作",
    key: "actions",
    width: 110,
    render: (row) =>
      h(
        NButton,
        {
          size: "small",
          secondary: true,
          onClick: () => emit("viewLogs", row),
        },
        {
          icon: () => h(ScrollText),
          default: () => "日志",
        },
      ),
  },
]);

function statusTagType(status: string) {
  if (status === "succeeded") {
    return "success";
  }
  if (status === "failed") {
    return "error";
  }
  if (status === "running") {
    return "warning";
  }
  if (status === "pending") {
    return "info";
  }
  return "default";
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    pending: "排队中",
    running: "运行中",
    succeeded: "成功",
    failed: "失败",
    canceled: "已取消",
  };
  return labels[status] ?? status;
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

function fallbackID(id: number | null) {
  return id ? `#${id}` : "-";
}

function formatTime(value: string | null) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString();
}
</script>

<template>
  <NDataTable
    :columns="columns"
    :data="tasks"
    :loading="loading"
    :row-key="(row) => row.id"
    :pagination="{ pageSize: 12 }"
    :scroll-x="1670"
  />
</template>

<style scoped>
:deep(.n-data-table-td) {
  white-space: nowrap;
}
</style>
