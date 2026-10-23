<template>
  <div class="app-shell management-console">
    <aside class="sidebar"><slot name="sidebar" /></aside>
    <main class="main-shell"><slot name="main" /></main>
    <slot />
  </div>
</template>
