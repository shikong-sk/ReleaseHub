<script setup lang="ts">
import { NConfigProvider, NMessageProvider } from 'naive-ui'

import MainLayout from './layouts/MainLayout.vue'
</script>

<template>
  <NConfigProvider>
    <NMessageProvider>
      <MainLayout />
    </NMessageProvider>
  </NConfigProvider>
</template>
