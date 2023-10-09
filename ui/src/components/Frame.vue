<template>
    <div class="mb-1rem">
        <template v-if="selectedLogGroup === null">
            <div class="mb-0_5rem">{{ logGroups.length }} records</div>
            <div class="two-column-table">
                <a
                    class="two-column-table-row cursor-pointer remove-anchor-styles"
                    v-for="logGroup in logGroups"
                    :href="getLogGroupHref(logGroup)"
                    @click.prevent="selectedLogGroup = logGroup.group"
                >
                    <div>{{ formatDate(logGroup.lastEventTime) }}</div>
                    <div>{{ logGroup.group }}</div>
                </a>
            </div>
        </template>
        <div v-else>
            Log Group: {{ selectedLogGroup }} <button @click="selectedLogGroup = null; selectedLogStream = null;">Back</button>
        </div>
    </div>

    <div class="mb-1rem" v-if="selectedLogGroup !== null">
        <template v-if="selectedLogStream === null">
            <div class="mb-0_5rem">{{ logStreams.length }} records</div>
            <form class="mb-0_5rem" @submit.prevent="getLogStreams">
                <input type="search" v-model="search">
                <button class="ml-0_5rem">Search</button>
            </form>
            <div class="two-column-table">
                <a
                    class="two-column-table-row cursor-pointer remove-anchor-styles"
                    v-for="logStream in logStreams"
                    :href="getLogStreamHref(logStream)"
                    @click.prevent="selectedLogStream = logStream.stream"
                >
                    <div>{{ formatDate(logStream.lastEventTime) }}</div>
                    <div>{{ logStream.stream }}</div>
                </a>
            </div>
        </template>
        <div v-else>
            Log Stream: {{ selectedLogStream }} <button @click="selectedLogStream = null">Back</button>
        </div>
    </div>

    <div class="mb-1rem" v-if="selectedLogStream !== null">
        <div class="mb-0_5rem">{{ logs.length }} records</div>
        <div class="two-column-table">
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
const firstLoad = ref(true)
const search = ref('')

function formatDate(date) {
    return dayjs(date).format('DD-MMM-YY hh:mm:ss A')
}

async function getLogGroups() {
    console.log('Fetching log groups')
    const response = await fetch('/log')
    const data = await response.json()
    logGroups.value = data
}

async function getLogStreams() {
    console.log('Fetching log streams')
    const response = await fetch(`/log?group=${selectedLogGroup.value}&search=${search.value}`)
    const data = await response.json()
    logStreams.value = data
}

async function getLogs() {
    console.log('Fetching logs')
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

async function loadQueryParams() {
    const queryParams = new URLSearchParams(window.location.search)
    const group = queryParams.get('group')
    const stream = queryParams.get('stream')

    if (group === null) {
        await getLogGroups()
        firstLoad.value = false
        return
    }

    if(group !== null) {
        selectedLogGroup.value = group
        if (stream === null) {
            await getLogStreams()
            firstLoad.value = false
            return
        }
    }

    if(stream !== null) {
        selectedLogStream.value = stream
        await getLogs()
        firstLoad.value = false
    }
}

function getLogGroupHref(logGroup) {
    const queryObject = {
        group: logGroup.group
    }

    const queryParams = new URLSearchParams(queryObject).toString()
    return `?${queryParams}`
}

function getLogStreamHref(logStream) {
    const queryObject = {
        group: selectedLogGroup.value,
        stream: logStream.stream
    }

    const queryParams = new URLSearchParams(queryObject).toString()
    return `?${queryParams}`
}

watch(selectedLogGroup, () => {
    console.log('selectedLogGroup changed', selectedLogGroup.value)
    if (firstLoad.value === true) {
        console.log('selectedLogGroup change skipped: firstLoad')
        return
    }
    setQueryParams()
    if (selectedLogGroup.value === null) {
        logStreams.value = []
        getLogGroups()
        return
    }
    getLogStreams()
})

watch(selectedLogStream, () => {
    console.log('selectedLogStream changed', selectedLogStream.value)
    if (firstLoad.value === true) {
        console.log('selectedLogStream change skipped: firstLoad')
        return
    }
    setQueryParams()
    if (selectedLogStream.value === null) {
        logs.value = []
        getLogStreams()
        return
    }
    getLogs()
})

onBeforeMount(() => {
    loadQueryParams()
})
</script>
