<template>
  <div id="app" class="min-h-screen">
    <router-view />
    <DialogHost />
  </div>
</template>

<script setup lang="ts">
import DialogHost from './components/DialogHost.vue'
</script>
