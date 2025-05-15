import { Client, GatewayIntentBits, EmbedBuilder, ActionRowBuilder, ButtonBuilder, ButtonStyle, WebhookClient } from 'discord.js';
import { exec } from 'child_process';
import { promisify } from 'util';
import dotenv from 'dotenv';
import path from 'path';
import { fileURLToPath } from 'url';
import crypto from 'crypto';

dotenv.config();
const execPromise = promisify(exec);

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const projectDir = __dirname;

const { DISCORD_TOKEN, CHANNEL_ID, CHANNEL_ID_2, SERVER_NAME, DISCORD_WEBHOOK } = process.env;

if (!DISCORD_TOKEN || !CHANNEL_ID || !SERVER_NAME) {
  throw new Error('Missing required environment variables');
}

const locationPrefix = SERVER_NAME.toLowerCase().replace(/\s+/g, '-');
const START_MIDCAP_BUTTON_ID = `start-midcap-${locationPrefix}`;
const STOP_MIDCAP_BUTTON_ID = `stop-midcap-${locationPrefix}`;
const START_LASTCAP_BUTTON_ID = `start-lastcap-${locationPrefix}`;
const STOP_LASTCAP_BUTTON_ID = `stop-lastcap-${locationPrefix}`;

// Track last log hash to detect new logs
const lastLogHashes = {
  'hll-geofences-midcap': '',
  'hll-geofences-lastcap': ''
};

const client = new Client({
  intents: [
    GatewayIntentBits.Guilds,
    GatewayIntentBits.GuildMessages,
    GatewayIntentBits.GuildMessageReactions
  ]
});

let webhookClient;
if (DISCORD_WEBHOOK) {
  try {
    webhookClient = new WebhookClient({ url: DISCORD_WEBHOOK });
  } catch (error) {
    console.error(`Error initializing webhook client: ${error.message}`);
  }
}

async function isDockerRunning() {
  try {
    const containers = [
      { name: 'hll-geofences-midcap', service: 'hll-geofences-midcap', status: false },
      { name: 'hll-geofences-lastcap', service: 'hll-geofences-lastcap', status: false }
    ];

    for (const container of containers) {
      const { stdout } = await execPromise(`docker ps -q -f name=^${container.name}$`);
      container.status = stdout.trim().length > 0;
    }

    return containers;
  } catch (error) {
    console.error(`Error checking Docker status: ${error.message}`);
    return [
      { name: 'hll-geofences-midcap', service: 'hll-geofences-midcap', status: false },
      { name: 'hll-geofences-lastcap', service: 'hll-geofences-lastcap', status: false }
    ];
  }
}

async function fetchNewLogs(service) {
  try {
    const { stdout } = await execPromise(`cd ${projectDir} && docker compose -p hll-geofences-midcap logs --since=1m ${service}`);
    const logs = stdout || 'No new logs available';
    
    // Calculate hash of logs to detect changes
    const logHash = crypto.createHash('md5').update(logs).digest('hex');
    
    if (logHash === lastLogHashes[service] || logs === 'No new logs available') {
      return null; // No new logs
    }
    
    lastLogHashes[service] = logHash;
    
    // Truncate logs to fit Discord's 2000-character limit
    if (logs.length > 1900) {
      return logs.slice(0, 1900) + '... (truncated)';
    }
    
    return logs;
  } catch (error) {
    console.error(`Error fetching logs for ${service}: ${error.message}`);
    return `Error fetching logs for ${service}: ${error.message}`;
  }
}

async function sendPeriodicLogs() {
  if (!webhookClient) return;

  const containers = await isDockerRunning();
  for (const container of containers) {
    if (container.status) {
      const logs = await fetchNewLogs(container.service);
      if (logs) {
        try {
          await webhookClient.send({
            content: `New logs for ${container.service}:\n\`\`\`\n${logs}\n\`\`\``,
            username: `${SERVER_NAME} Seeding Bot`
          });
        } catch (error) {
          console.error(`Error sending logs to webhook for ${container.service}: ${error.message}`);
        }
      }
    }
  }
}

async function createEmbed() {
  const containers = await isDockerRunning();
  const isAnyRunning = containers.some(c => c.status);

  const embed = new EmbedBuilder()
    .setTitle('HLL Seeding Bot')
    .setDescription('Midcap <50 Player | Lastcap <70 Player')
    .setColor(isAnyRunning ? 0x00FF00 : 0xFF0000)
    .setFooter({ text: `Server: ${SERVER_NAME}` })
    .setTimestamp();

  for (const container of containers) {
    embed.addFields({
      name: `Container: ${container.name}`,
      value: container.status ? '🟢 Running' : '🔴 Stopped'
    });
  }

  return embed;
}

function createButtons(containers) {
  const midcapRunning = containers.find(c => c.service === 'hll-geofences-midcap').status;
  const lastcapRunning = containers.find(c => c.service === 'hll-geofences-lastcap').status;

  return [
    new ActionRowBuilder()
      .addComponents(
        new ButtonBuilder()
          .setCustomId(START_MIDCAP_BUTTON_ID)
          .setLabel('START MIDCAP')
          .setStyle(ButtonStyle.Success)
          .setDisabled(midcapRunning),
        new ButtonBuilder()
          .setCustomId(STOP_MIDCAP_BUTTON_ID)
          .setLabel('STOP MIDCAP')
          .setStyle(ButtonStyle.Danger)
          .setDisabled(!midcapRunning)
      ),
    new ActionRowBuilder()
      .addComponents(
        new ButtonBuilder()
          .setCustomId(START_LASTCAP_BUTTON_ID)
          .setLabel('START LASTCAP')
          .setStyle(ButtonStyle.Success)
          .setDisabled(lastcapRunning),
        new ButtonBuilder()
          .setCustomId(STOP_LASTCAP_BUTTON_ID)
          .setLabel('STOP LASTCAP')
          .setStyle(ButtonStyle.Danger)
          .setDisabled(!lastcapRunning)
      )
  ];
}

async function clearChannel(channel) {
  try {
    const messages = await channel.messages.fetch({ limit: 100 });
    if (messages.size > 0) {
      await channel.bulkDelete(messages);
    }
  } catch (error) {
    console.error(`Error clearing channel ${channel.id}: ${error.message}`);
  }
}

async function updateEmbedInChannel(channel) {
  try {
    const embed = await createEmbed();
    const containers = await isDockerRunning();
    const buttons = createButtons(containers);
    const messages = await channel.messages.fetch({ limit: 1 });
    const message = messages.first();

    if (message) {
      await message.edit({ embeds: [embed], components: buttons });
    } else {
      await channel.send({ embeds: [embed], components: buttons });
    }
  } catch (error) {
    console.error(`Error updating embed in channel ${channel.id}: ${error.message}`);
  }
}

async function updateEmbed(channels) {
  for (const channel of channels) {
    await updateEmbedInChannel(channel);
  }
}

async function executeDockerCommand(service, action, interaction) {
  try {
    if (!interaction.deferred && !interaction.replied) {
      await interaction.deferReply({ ephemeral: true });
    }

    const projectName = 'hll-geofences-midcap';
    let command;
    if (action === 'start') {
      command = `cd ${projectDir} && docker compose -p ${projectName} up -d ${service}`;
    } else if (action === 'stop') {
      command = `cd ${projectDir} && docker compose -p ${projectName} stop ${service}`;
    } else {
      throw new Error('Invalid action');
    }

    const { stdout, stderr } = await execPromise(command);
    const output = stdout || stderr || 'No output';

    // Fetch Docker Compose logs if webhook is configured
    if (webhookClient) {
      try {
        const logs = await fetchNewLogs(service);
        if (logs) {
          await webhookClient.send({
            content: `Logs for ${service} (${action}):\n\`\`\`\n${logs}\n\`\`\``,
            username: `${SERVER_NAME} Seeding Bot`
          });
        }
      } catch (logsError) {
        console.error(`Error fetching logs for ${service}: ${logsError.message}`);
        await webhookClient.send({
          content: `Error fetching logs for ${service} (${action}): ${logsError.message}`,
          username: `${SERVER_NAME} Seeding Bot`
        });
      }
    }

    await interaction.editReply({
      content: `Command executed successfully for ${service}:\n\`\`\`\n${output.trim()}\n\`\`\``
    });

    const channels = [interaction.channel];
    if (CHANNEL_ID_2) {
      const channel2 = await client.channels.fetch(CHANNEL_ID_2);
      if (channel2) channels.push(channel2);
    }
    await updateEmbed(channels);
  } catch (error) {
    console.error(`Error executing command for ${service}: ${error.message}`);
    if (!interaction.deferred && !interaction.replied) {
      await interaction.deferReply({ ephemeral: true });
    }
    await interaction.editReply({
      content: `Error executing command for ${service}: ${error.message}`
    });
  }
}

client.once('ready', async () => {
  console.log(`Bot started for ${SERVER_NAME}`);

  const channels = [];
  const channel1 = await client.channels.fetch(CHANNEL_ID);
  if (channel1) channels.push(channel1);

  if (CHANNEL_ID_2) {
    try {
      const channel2 = await client.channels.fetch(CHANNEL_ID_2);
      if (channel2) channels.push(channel2);
    } catch (error) {
      console.error(`Error fetching CHANNEL_ID_2: ${error.message}`);
    }
  }

  for (const channel of channels) {
    await clearChannel(channel);
    await updateEmbedInChannel(channel);
  }

  // Periodic embed update
  setInterval(async () => {
    await updateEmbed(channels);
  }, 60000);

  // Periodic log check
  setInterval(async () => {
    await sendPeriodicLogs();
  }, 60000);
});

client.on('interactionCreate', async (interaction) => {
  if (!interaction.isButton()) return;

  if (interaction.customId === START_MIDCAP_BUTTON_ID) {
    await executeDockerCommand('hll-geofences-midcap', 'start', interaction);
  } else if (interaction.customId === STOP_MIDCAP_BUTTON_ID) {
    await executeDockerCommand('hll-geofences-midcap', 'stop', interaction);
  } else if (interaction.customId === START_LASTCAP_BUTTON_ID) {
    await executeDockerCommand('hll-geofences-lastcap', 'start', interaction);
  } else if (interaction.customId === STOP_LASTCAP_BUTTON_ID) {
    await executeDockerCommand('hll-geofences-lastcap', 'stop', interaction);
  }
});

client.login(DISCORD_TOKEN);

process.on('SIGTERM', async () => {
  console.log('Shutting down...');
  await client.destroy();
  process.exit(0);
});
