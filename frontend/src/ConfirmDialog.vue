<template>
  <v-dialog :model-value="state.open" max-width="440" @update:model-value="(v) => { if (!v) cancel() }">
    <v-card rounded="xl" elevation="8">
      <v-card-item class="pb-2">
        <div class="d-flex align-center mb-1">
          <v-avatar color="confirm-avatar" size="38" rounded="lg" class="mr-3">
            <v-icon :icon="state.icon" size="22" />
          </v-avatar>
          <v-card-title class="text-h6 px-0">{{ state.title }}</v-card-title>
        </div>
      </v-card-item>

      <v-card-text class="text-body-2 text-medium-emphasis confirm-msg">
        {{ state.message }}
      </v-card-text>

      <v-card-actions class="px-5 pb-5 pt-1 flex-wrap">
        <v-spacer />
        <v-btn variant="text" @click="cancel">{{ state.cancelText }}</v-btn>
        <v-btn v-if="state.extraText" variant="tonal" color="error" class="ml-2" @click="extra">
          {{ state.extraText }}
        </v-btn>
        <v-btn :color="state.color" variant="flat" class="ml-2" @click="confirm">
          {{ state.confirmText }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup>
import { useConfirmState } from './confirm'

const { state, confirm, cancel, extra } = useConfirmState()
</script>

<style scoped>
.confirm-msg {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.6;
}
.confirm-avatar {
  background: rgba(var(--v-theme-error), 0.12);
  color: rgb(var(--v-theme-error));
}
</style>
