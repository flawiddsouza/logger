<template>
    <div class="mb-1rem">
        <div class="two-column-table" v-if="selectedLogGroup === null">
            <div class="two-column-table-row cursor-pointer" v-for="logGroup in logGroups" @click="selectedLogGroup = logGroup.group">
                <div>{{ formatDate(logGroup.lastEventTime) }}</div>
                <div>{{ logGroup.group }}</div>
            </div>
        </div>
        <div v-else>
            Log Group: {{ selectedLogGroup }} <button @click="selectedLogGroup = null; selectedLogStream = null;">Back</button>
        </div>
    </div>

    <div class="mb-1rem">
        <div class="two-column-table" v-if="selectedLogStream === null">
            <div class="two-column-table-row cursor-pointer" v-for="logStream in logStreams" @click="selectedLogStream = logStream.stream">
                <div>{{ formatDate(logStream.lastEventTime) }}</div>
                <div>{{ logStream.stream }}</div>
            </div>
        </div>
        <div v-else>
            Log Stream: {{ selectedLogStream }} <button @click="selectedLogStream = null">Back</button>
        </div>
    </div>

    <div class="mb-1rem">
        <div class="two-column-table" v-if="selectedLogStream !== null">
            <div class="two-column-table-row" v-for="log in logs">
                <div>{{ formatDate(log.timestamp) }}</div>
                <div>{{ log.message }}</div>
            </div>
        </div>
    </div>
</template>

<script setup>
import { ref, watch, onBeforeMount } from 'vue'
import dayjs from 'dayjs'

const logGroups = ref([])
const selectedLogGroup = ref(null)
const logStreams = ref([])
const selectedLogStream = ref(null)
const logs = ref([])

function formatDate(date) {
    return dayjs(date).format('DD-MMM-YY hh:mm:ss A')
}

async function getLogGroups() {
    const response = await fetch('/log')
    const data = await response.json()
    logGroups.value = data
}

async function getLogStreams() {
    const response = await fetch(`/log?group=${selectedLogGroup.value}`)
    const data = await response.json()
    logStreams.value = data
}

async function getLogs() {
    const response = await fetch(`/log?group=${selectedLogGroup.value}&stream=${selectedLogStream.value}`)
    const data = await response.json()
    logs.value = data
}

function setQueryParams() {
    const queryObject = {
        group: selectedLogGroup.value ?? undefined,
        stream: selectedLogStream.value ?? undefined
    }

    Object.keys(queryObject).forEach(key => queryObject[key] === undefined && delete queryObject[key])

    const queryParams = new URLSearchParams(queryObject).toString()
    history.pushState(null, null, queryParams.length ? `?${queryParams}` : '/')
}

function loadQueryParams() {
    const queryParams = new URLSearchParams(window.location.search)
    const group = queryParams.get('group')
    const stream = queryParams.get('stream')
    if(group !== null) {
        selectedLogGroup.value = group
    }
    if(stream !== null) {
        selectedLogStream.value = stream
    }
}

watch(selectedLogGroup, () => {
    setQueryParams()
    if (selectedLogGroup.value === null) {
        logStreams.value = []
        return
    }
    getLogStreams()
})

watch(selectedLogStream, () => {
    setQueryParams()
    if (selectedLogStream.value === null) {
        logs.value = []
        return
    }
    getLogs()
})

onBeforeMount(() => {
    getLogGroups()
    loadQueryParams()
})
</script>
